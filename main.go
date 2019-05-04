package main

import (
	"flag"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"os"
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
	settings := SettingsStruct{}

	flag.StringVar(
		&settings.TelegramToken,
		"token",
		"",
		"Telegram bot API token",
	)

	flag.StringVar(
		&settings.CachePath,
		"cachepath",
		"/tmp/.eis_search_bot_cache",
		"Cache directory path",
	)

	flag.IntVar(
		&settings.CacheSize,
		"cachesize",
		500,
		"Max cached files number",
	)

	flag.IntVar(
		&settings.BotMaxResults,
		"maxresults",
		30,
		"Limit of results provided by bot for one search request",
	)

	flag.Parse()

	if settings.TelegramToken == "" {
		settings.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
		if settings.TelegramToken == "" {
			fmt.Fprintln(
				os.Stderr,
				"Telegram bot API token is not provided. Specify -token=TOKEN or TELEGRAM_TOKEN environment variable.",
			)
			panic("token is required")
		}
	}

	return &settings
}
