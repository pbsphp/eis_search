package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"os"
)

func main() {
	tgToken := os.Getenv("TG_TOKEN")
	if tgToken == "" {
		fmt.Println("Please set TG_TOKEN env variable.")
		panic("Empty TG_TOKEN.")
	}

	bot, err := tgbotapi.NewBotAPI(tgToken)
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
