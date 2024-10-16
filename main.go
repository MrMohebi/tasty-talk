package main

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	adminID := chatID(os.Getenv("ADMIN_ID"))

	bot, err := telego.NewBot(botToken, telego.WithDefaultDebugLogger())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	botUser, err := bot.GetMe()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("Bot user: %+v\n", botUser)

	updates, _ := bot.UpdatesViaLongPolling(nil)

	bh, _ := th.NewBotHandler(bot, updates)

	// user chat location handler
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		chatID := tu.ID(update.Message.Chat.ID)
		message := update.Message

		address, err := locationAddress(message.Location.Latitude, message.Location.Longitude)
		if err != nil {
			println(err)
		}

		_, _ = bot.SendMessage(tu.Message(chatID, "آدرس: "+address+"\n\n"+"ثبت شد").WithReplyMarkup(tu.ReplyKeyboardRemove()))

	},
		th.AnyMessage(),
		func(update telego.Update) bool {
			chatID := tu.ID(update.Message.Chat.ID)
			return chatID.String() != adminID.String() && update.Message.Location != nil
		},
	)

	// user chat contact handler
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		chatID := tu.ID(update.Message.Chat.ID)
		message := update.Message

		_, _ = bot.SendMessage(tu.Message(chatID, "مخاطب: "+message.Contact.FirstName+" "+message.Contact.LastName+"\n\n"+"ثبت شد").WithReplyMarkup(tu.ReplyKeyboardRemove()))

	},
		th.AnyMessage(),
		func(update telego.Update) bool {
			chatID := tu.ID(update.Message.Chat.ID)
			return chatID.String() != adminID.String() && update.Message.Contact != nil
		},
	)

	// user chat handler
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		chatID := tu.ID(update.Message.Chat.ID)
		message := update.Message

		inlineKeyboard := tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("د موقعیت").WithCallbackData("location_"+chatID.String()),
				tu.InlineKeyboardButton("د شماره").WithCallbackData("phone_"+chatID.String()),
			),
			tu.InlineKeyboardRow( // Row 1
				tu.InlineKeyboardButton("ادرس ها").WithCallbackData("userAddresses_"+strconv.FormatInt(message.From.ID, 64)),
				tu.InlineKeyboardButton("شماره ها").WithCallbackData("userContacts_"+strconv.FormatInt(message.From.ID, 64)),
			),
		)

		baseEntries := []tu.MessageEntityCollection{tu.Entity("\n\n"), tu.Entity("#id_" + chatID.String()).Hashtag()}

		// copy message to admin group
		m := tu.CopyMessage(adminID, chatID, message.MessageID)
		mID, _ := bot.CopyMessage(m)

		if message.Text != "" {
			baseEntries = slices.Insert(baseEntries, 0, tu.Entity(message.Text))
			t, e := tu.MessageEntities(baseEntries...)
			_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:      m.ChatID,
				MessageID:   mID.MessageID,
				Text:        t,
				Entities:    e,
				ReplyMarkup: inlineKeyboard,
			})
		} else {
			t, e := tu.MessageEntities(baseEntries...)
			_, _ = bot.EditMessageCaption(&telego.EditMessageCaptionParams{
				ChatID:          m.ChatID,
				MessageID:       mID.MessageID,
				Caption:         t,
				CaptionEntities: e,
				ReplyMarkup:     inlineKeyboard,
			})
		}

	},
		th.AnyMessage(),
		func(update telego.Update) bool {
			chatID := tu.ID(update.Message.Chat.ID)
			return chatID.String() != adminID.String()
		},
	)

	// admin chat handler
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		chatID := tu.ID(update.Message.Chat.ID)
		message := update.Message
		if message.ReplyToMessage != nil {
			text := ""
			if message.ReplyToMessage.Text != "" {
				text = message.ReplyToMessage.Text
			} else {
				text = message.ReplyToMessage.Caption
			}

			m := tu.CopyMessage(extractIDFromMessage(text), chatID, message.MessageID)
			_, _ = bot.CopyMessage(m)
		}
	},
		th.AnyMessage(),
		func(update telego.Update) bool {
			chatID := tu.ID(update.Message.Chat.ID)
			return chatID.String() == adminID.String()
		},
	)

	bh.HandleCallbackQuery(func(bot *telego.Bot, query telego.CallbackQuery) {
		queryKeys := strings.Split(query.Data, "_")
		action := queryKeys[0]
		chatId := chatID(queryKeys[1])
		addressId := ""
		if len(queryKeys) > 2 {
			addressId = queryKeys[2]
		}

		println(addressId)

		switch action {
		case "location":
			inlineKeyboard := tu.InlineKeyboard(
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("عنوان ادرس یک").WithCallbackData("userAddress_"+chatId.String()+"_1"),
					tu.InlineKeyboardButton("عنوان ادرس دو").WithCallbackData("userAddress_"+chatId.String()+"_2"),
				),
			)
			keyboard := tu.Keyboard(
				tu.KeyboardRow( // Row 1
					tu.KeyboardButton("ارسال موقعیت مکانی فعلی").WithRequestLocation(),
				),
			).WithResizeKeyboard().WithOneTimeKeyboard()

			_, _ = bot.SendMessage(tu.Message(chatId, "آدرس خود را انتخاب کنید.").WithReplyMarkup(inlineKeyboard))
			_, _ = bot.SendMessage(tu.Message(chatId, "میتوانید از دکمه زیر برای ارسال موقعیت فعلی خود استفاده کنید. برای ارسال ادرس متفاوت موقعیت مکانی دلخواه را از بخش پیوست ها انتخاب کنید").WithReplyMarkup(keyboard))
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("address request sent!"))
		case "phone":
			keyboard := tu.Keyboard(
				tu.KeyboardRow( // Row 1
					tu.KeyboardButton("ارسال شماره").WithRequestContact(),
				),
			).WithResizeKeyboard().WithOneTimeKeyboard()

			_, _ = bot.SendMessage(tu.Message(chatId, "مخاطب گیرنده را ارسال کنید.").WithReplyMarkup(keyboard))

			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("phone request sent!"))
		case "userAddress":
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("ok userAddress"))
		case "userAddresses":
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("addresses  of " + chatId.String()))
		default:
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("no query found!"))
		}
	})

	defer bh.Stop()
	defer bot.StopLongPolling()

	bh.Start()
}

func locationAddress(lat float64, lon float64) (string, error) {
	type DigikalaRes struct {
		Status int `json:"status"`
		Data   struct {
			Address struct {
				Address string `json:"address"`
				StateID int    `json:"state_id"`
				CityID  int    `json:"city_id"`
			} `json:"address"`
		} `json:"data"`
	}

	base, _ := url.Parse("https://api.digikala.com/v1/map/reverse-geo/")

	params := url.Values{}
	params.Add("latitude", strconv.FormatFloat(lat, 'f', -1, 64))
	params.Add("longitude", strconv.FormatFloat(lon, 'f', -1, 64))
	base.RawQuery = params.Encode()

	res, err := sendGetRequest(base.String())
	if err != nil {
		return "", err
	}
	data := DigikalaRes{}
	err = json.Unmarshal([]byte(res), &data)
	if err != nil {
		return "", err
	}

	return data.Data.Address.Address, nil
}
func sendGetRequest(url string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}
	return string(body), nil
}

func chatID(id string) telego.ChatID {
	ID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		println("Error in converting chatId =>", id)
	}

	return tu.ID(ID)
}

func extractIDFromMessage(text string) telego.ChatID {
	id := ""

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, "#id_") {
			id = strings.Split(line, "#id_")[1]
		}
	}

	return chatID(id)
}
