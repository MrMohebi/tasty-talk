package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

type User struct {
	TelegramId  int64
	Phone       sql.NullString
	Username    sql.NullString
	Addresses   sql.NullString
	Contacts    sql.NullString
	CreatedAt   sql.NullTime
	lastOrderAt sql.NullTime
}

type Address struct {
	Id        int32
	Latitude  float64
	Longitude float64
	TextAuto  string
	Text      string
	Title     string
}

type Contact struct {
	Id        int32
	Name      string
	Firstname string
	Lastname  string
	Phone     string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	adminID := chatID(os.Getenv("ADMIN_ID"))

	db, err := sql.Open("sqlite3", "./db.sqlite3")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("create table if not exists users (telegram_id INTEGER not null,phone TEXT, username TEXT, addresses TEXT, contacts TEXT, created_at TIMESTAMP default CURRENT_TIMESTAMP not null, last_order_at TIMESTAMP, CONSTRAINT `telegram_id_UNIQUE` PRIMARY KEY (telegram_id) );")
	if err != nil {
		log.Fatal(err)
	}

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

	// Register handler with match on command `/start`
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		var user User
		user.TelegramId = update.Message.From.ID
		user.Username = sql.NullString{String: update.Message.From.Username}

		err = insertUser(db, &user)
		if err != nil {
			println(err)
		}
		// Send message
		_, _ = bot.SendMessage(tu.Messagef(
			tu.ID(update.Message.Chat.ID),
			"Hello %s!", update.Message.From.FirstName,
		))
	}, th.CommandEqual("start"))

	// user chat location handler
	bh.Handle(func(bot *telego.Bot, update telego.Update) {
		chatID := tu.ID(update.Message.Chat.ID)
		message := update.Message

		address, err := locationAddress(message.Location.Latitude, message.Location.Longitude)
		if err != nil {
			println(err)
		}

		addr := Address{Id: rune(randNumber(5)), Text: address, TextAuto: address, Latitude: message.Location.Latitude, Longitude: message.Location.Longitude}
		var user User
		if err := getUser(db, &user, message.From.ID); err != nil {
			println(err)
			return
		}

		if err := addAddressToUser(db, &user, addr); err != nil {
			println(err)
			return
		}

		_, _ = bot.SendMessage(tu.Message(chatID, "آدرس: "+address+"\n\n"+"ثبت شد").WithReplyMarkup(tu.ReplyKeyboardRemove()))

		inlineKeyboard := tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("د موقعیت").WithCallbackData("location_"+chatID.String()),
				tu.InlineKeyboardButton("د شماره").WithCallbackData("phone_"+chatID.String()),
			),
			tu.InlineKeyboardRow( // Row 1
				tu.InlineKeyboardButton("ادرس ها").WithCallbackData("userAddresses_"+strconv.FormatInt(message.From.ID, 10)),
				tu.InlineKeyboardButton("شماره ها").WithCallbackData("userContacts_"+strconv.FormatInt(message.From.ID, 10)),
			),
		)
		entries := []tu.MessageEntityCollection{tu.Entity("آدرس جدیدی ثبت کرد"), tu.Entity("\n"), tu.Entity("#id_" + chatID.String()).Hashtag()}
		_, _ = bot.SendMessage(tu.MessageWithEntities(adminID, entries...).WithReplyMarkup(inlineKeyboard))

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

		contact := Contact{Id: rune(randNumber(5)), Firstname: message.Contact.FirstName, Lastname: message.Contact.LastName, Name: "", Phone: message.Contact.PhoneNumber}
		var user User
		if err := getUser(db, &user, message.From.ID); err != nil {
			println(err)
			return
		}
		if err := addContactToUser(db, &user, contact); err != nil {
			println(err)
			return
		}

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
				tu.InlineKeyboardButton("ادرس ها").WithCallbackData("userAddresses_"+strconv.FormatInt(message.From.ID, 10)),
				tu.InlineKeyboardButton("شماره ها").WithCallbackData("userContacts_"+strconv.FormatInt(message.From.ID, 10)),
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

		var addressId int32
		if len(queryKeys) > 2 {
			ad, _ := strconv.ParseInt(queryKeys[2], 10, 32)
			addressId = int32(ad)
		}

		switch action {
		case "location":
			var user User
			if err := getUser(db, &user, query.From.ID); err != nil {
				println(err)
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("err!"))
			}

			address := parseAddresses(user.Addresses.String)

			inlineKeyboardRows := userAddressesAsInlineButtons(address, chatId, "userAddress")

			inlineKeyboard := tu.InlineKeyboard(inlineKeyboardRows...)

			keyboard := tu.Keyboard(
				tu.KeyboardRow(
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
			var user User
			if err := getUser(db, &user, query.From.ID); err != nil {
				println(err)
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("err!"))
			}

			addresses := parseAddresses(user.Addresses.String)

			var selected Address
			for _, item := range addresses {
				if item.Id == addressId {
					selected = item
				}
			}

			entries := []tu.MessageEntityCollection{tu.Entity("آدرس انتخاب شده کابر"), tu.Entity("\n"), tu.Entity(selected.Text), tu.Entity("\n"), tu.Entity("#id_" + strconv.FormatInt(query.From.ID, 10)).Hashtag()}

			m, _ := bot.SendLocation(tu.Location(adminID, selected.Latitude, selected.Longitude))
			_, _ = bot.SendMessage(tu.MessageWithEntities(adminID, entries...).WithReplyParameters(&telego.ReplyParameters{MessageID: m.MessageID}))

			_, _ = bot.SendMessage(tu.Messagef(tu.ID(query.From.ID), "ادرس %s. با موفقیت ثبت شد", selected.Text).WithReplyMarkup(tu.ReplyKeyboardRemove()))
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("ثبت شد"))
		case "userAddresses":
			var user User
			if err := getUser(db, &user, query.From.ID); err != nil {
				println(err)
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("err!"))
			}
			data := parseAddresses(user.Addresses.String)
			for _, item := range data {
				m, _ := bot.SendLocation(tu.Location(adminID, item.Latitude, item.Longitude).WithReplyParameters(&telego.ReplyParameters{MessageID: query.Message.GetMessageID()}))
				_, _ = bot.SendMessage(tu.Message(adminID, item.Text).WithReplyParameters(&telego.ReplyParameters{MessageID: m.MessageID}))
			}
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("addresses of " + chatId.String()))
		case "userContacts":
			var user User
			if err := getUser(db, &user, query.From.ID); err != nil {
				println(err)
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("err!"))
			}
			data := parseContacts(user.Contacts.String)
			for _, item := range data {
				_, _ = bot.SendContact(tu.Contact(adminID, item.Phone, item.Firstname).WithReplyParameters(&telego.ReplyParameters{MessageID: query.Message.GetMessageID()}))
			}
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("contacts  of " + chatId.String()))
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

func insertUser(db *sql.DB, user *User) error {
	stmt, err := db.Prepare("INSERT OR IGNORE INTO users(telegram_id, username) VALUES(?, ?);")
	if err != nil {
		return err
	}

	defer stmt.Close()
	_, err = stmt.Exec(user.TelegramId, user.Username)
	if err != nil {
		return err
	}
	return nil
}

func getUser(db *sql.DB, user *User, telegramId int64) error {
	stmt, err := db.Prepare("select * from users where telegram_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = stmt.QueryRow(telegramId).Scan(&user.TelegramId, &user.Phone, &user.Username, &user.Addresses, &user.Contacts, &user.CreatedAt, &user.lastOrderAt)
	if err != nil {
		return err
	}

	return nil
}

func parseAddresses(address string) []Address {
	var data []Address
	if address != "" && len(address) > 1 {
		err := json.Unmarshal([]byte(address), &data)
		if err != nil {
			println(err)
			return nil
		}
	}
	return data
}

func parseContacts(contact string) []Contact {
	var data []Contact
	if contact != "" && len(contact) > 1 {
		err := json.Unmarshal([]byte(contact), &data)
		if err != nil {
			println(err)
			return nil
		}
	}
	return data
}

func addAddressToUser(db *sql.DB, user *User, address Address) error {
	stmt, err := db.Prepare("UPDATE users SET addresses = ? WHERE telegram_id = ?")
	if err != nil {
		return err
	}
	var addresses []Address
	if user.Addresses.Valid && len(user.Addresses.String) > 2 {
		addresses = parseAddresses(user.Addresses.String)
	}

	addresses = append(addresses, address)

	data, err := json.Marshal(addresses)
	if err != nil {
		return err
	}

	defer stmt.Close()
	_, err = stmt.Exec(data, user.TelegramId)
	if err != nil {
		return err
	}
	return nil
}

func addContactToUser(db *sql.DB, user *User, contact Contact) error {
	stmt, err := db.Prepare("UPDATE users SET contacts = ? WHERE telegram_id = ?")
	if err != nil {
		return err
	}
	var contacts []Contact
	if user.Contacts.Valid && len(user.Contacts.String) > 2 {
		contacts = parseContacts(user.Contacts.String)
	}

	contacts = append(contacts, contact)

	data, err := json.Marshal(contacts)
	if err != nil {
		return err
	}

	defer stmt.Close()
	_, err = stmt.Exec(data, user.TelegramId)
	if err != nil {
		return err
	}
	return nil
}

func userAddressesAsInlineButtons(addresses []Address, chatId telego.ChatID, action string) [][]telego.InlineKeyboardButton {
	addressesLen := len(addresses)
	sliceLen := 2
	items := int(math.Ceil(float64(addressesLen / sliceLen)))
	inlineKeyboardRows := make([][]telego.InlineKeyboardButton, items)
	for i := 1; i <= items; i++ {
		inlineKeyboardRows[i-1] = make([]telego.InlineKeyboardButton, sliceLen)
		for j := 1; j <= sliceLen; j++ {
			x := (i * j) - 1
			text := addresses[x].Text
			if len(addresses[x].Text) > 50 {
				text = addresses[x].Text[:50]
			}
			inlineKeyboardRows[i-1][j-1] = tu.InlineKeyboardButton(text).WithCallbackData(action + "_" + chatId.String() + "_" + StringInt32(addresses[x].Id))
		}
	}

	return inlineKeyboardRows
}

func randNumber(n int) int {

	minLimit := int(math.Pow10(n))
	maxLimit := int(math.Pow10(n - 1))
	randInt := int(rand.Float64() * float64(minLimit))
	if randInt < maxLimit {
		randInt += maxLimit
	}
	return randInt

}
func StringInt32(n int32) string {
	buf := [11]byte{}
	pos := len(buf)
	i := int64(n)
	signed := i < 0
	if signed {
		i = -i
	}
	for {
		pos--
		buf[pos], i = '0'+byte(i%10), i/10
		if i == 0 {
			if signed {
				pos--
				buf[pos] = '-'
			}
			return string(buf[pos:])
		}
	}
}
