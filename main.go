package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type SettingsStruct struct {
	TelegramToken string
	CachePath     string
	CacheSize     int
	BotMaxResults int
}

// Settings holder
var Settings = readSettings()

func main() {
	bot, err := tgbotapi.NewBotAPI(Settings.TelegramToken)
	if err != nil {
		panic(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)

	// Dictionary containing dialogs with users.
	// Key - Chat ID,
	// Value - dialog input channel.
	dialogsMap := make(map[int64]chan Message)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatId := update.Message.Chat.ID
		// Find existing dialog with this user. Or start it.
		var inbox, outbox chan Message
		inbox, ok := dialogsMap[chatId]
		if !ok {
			inbox, outbox = dialog()
			dialogsMap[chatId] = inbox
			// Postman will react on output dialog messages, send them to user and
			// destroy dialog when it is done.
			go postman(bot, outbox, dialogsMap, chatId)
		}

		inbox <- Message{
			Text: update.Message.Text,
		}
	}
}

// Helper. Panic if given error isn't nil.
func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

// Read and return settings. Panic on error.
func readSettings() *SettingsStruct {
	ex, err := os.Executable()
	checkError(err)
	currPath := filepath.Dir(ex)
	confPath := path.Join(currPath, "config.json")
	_, err = os.Stat(confPath)
	if os.IsNotExist(err) {
		fmt.Println("Please create config.json path and place it near executable binary")
		panic("missing config.json file")
	}
	dat, err := ioutil.ReadFile(path.Join(currPath, "config.json"))
	checkError(err)
	settings := SettingsStruct{}
	err = json.Unmarshal(dat, &settings)
	checkError(err)
	return &settings
}
