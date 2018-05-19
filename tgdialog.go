package main

import (
	"bytes"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"regexp"
	"strings"
	"time"
)

// Dialog message.
// simple object of message containing text, attached file, etc.
type Message struct {
	Text     string
	File     *bytes.Buffer
	FileName string
}

// Start simple dialog.
// Return input and output channels. Started goroutine listens for input messages from
// input channel and puts replies into output channel.
// When dialog is completed, output channel will be closed.
func dialog() (chan Message, chan Message) {
	inbox := make(chan Message)
	outbox := make(chan Message)

	// Listen for messages on input channel and put replies into output channel.
	// Close output channel on finish.
	go func() {
		for message := <-inbox; message.Text != "/start"; message = <-inbox {
			outbox <- Message{
				Text: "Используйте /start чтобы начать.",
			}
		}

		outbox <- Message{
			Text: "Что (где) искать?\n" +
				"Полный путь к директории или одно из: {контракт|извещение|ПГ|ПЗ}.",
		}
		message := <-inbox
		place := placeToDirectory(message.Text)

		for !strings.HasPrefix(place, "/") {
			outbox <- Message{
				Text: "Абсолютный путь (начинается на \"/\") или одно из: {контракт|извещение|ПГ|ПЗ}.",
			}
			message := <-inbox
			place = placeToDirectory(message.Text)
		}

		var filterFrom, filterTo string
		var ok bool
		for !ok {
			outbox <- Message{
				Text: "С какого по какое? (ДД.ММ.ГГГГ - ДД.ММ.ГГГГ).\n" +
					"Например для поиска за все время можно оставить \" - \".",
			}
			message = <-inbox
			filterFrom, filterTo, ok = parseDateFilters(message.Text)
		}

		outbox <- Message{
			Text: "Что искать? (Через запятую).",
		}
		message = <-inbox
		patterns := splitPatterns(message.Text)

		searchParams := SearchParams{
			Directory: place,
			FromDate:  filterFrom,
			ToDate:    filterTo,
			Patterns:  patterns,
		}

		if filterFrom == "" {
			filterFrom = "*"
		}
		if filterTo == "" {
			filterTo = "*"
		}
		outbox <- Message{
			Text: fmt.Sprintf(
				"Ищем %s в %s за период с %s по %s\nЧтобы отменить, введите /stop.",
				strings.Join(searchParams.Patterns, "|"),
				searchParams.Directory,
				filterFrom,
				filterTo,
			),
		}

		// Search and iterate over results.
		iterResults(&searchParams, inbox, outbox)

		close(outbox)
	}()

	return inbox, outbox
}

// Attach to dialog() output channel and send replies to telegram user.
// Destroy mailboxes (input and output returned by dialog()) when dialog is done.
// One goroutine for one dialog.
func postman(bot *tgbotapi.BotAPI, mailbox chan Message, dialogsMap map[int64]chan Message, chatId int64) {
	for message := range mailbox {
		if message.Text != "" {
			msg := tgbotapi.NewMessage(chatId, message.Text)
			bot.Send(msg)
		}
		if message.File != nil {
			msg := tgbotapi.NewDocumentUpload(chatId, tgbotapi.FileBytes{
				Name:  message.FileName,
				Bytes: message.File.Bytes(),
			})
			bot.Send(msg)
		}
	}

	delete(dialogsMap, chatId)
}

// Helper. Panic if given error isn't nil.
func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

// Converts given place shortnames to directories.
func placeToDirectory(place string) string {
	var directory string
	switch place {
	case "контракт", "Контракт":
		directory = "/fcs_regions/Tatarstan_Resp/contracts/"
	case "извещение", "Извещение":
		directory = "/fcs_regions/Tatarstan_Resp/notifications/"
	case "ПГ", "пг":
		directory = "/fcs_regions/Tatarstan_Resp/plangraphs2017/"
	case "ПЗ", "пз":
		directory = "/fcs_regions/Tatarstan_Resp/purchaseplans/"
	default:
		directory = place
	}
	return directory
}

// Parse dates from string given by user.
// String should be like this: "fromDate - toDate", where fromDate and toDate are
// optional dates (dd.mm.YYYY).
func parseDateFilters(userInput string) (string, string, bool) {
	matches := regexp.MustCompile("[ ]*-[ ]*").Split(userInput, 2)
	if matches == nil || len(matches) != 2 {
		return "", "", false
	}
	fromDateSrc := strings.TrimSpace(matches[0])
	toDateSrc := strings.TrimSpace(matches[1])
	var fromDate, toDate string
	if fromDateSrc != "" {
		t, err := time.Parse("02.01.2006", fromDateSrc)
		if err != nil { // Always check errors even if they should not happen.
			return "", "", false
		}
		fromDate = t.Format("20060102")
	}
	if toDateSrc != "" {
		t, err := time.Parse("02.01.2006", toDateSrc)
		if err != nil { // Always check errors even if they should not happen.
			return "", "", false
		}
		toDate = t.Format("20060102")
	}
	return fromDate, toDate, true
}

// Split comma separated string given by user to words.
func splitPatterns(userInput string) []string {
	split := strings.Split(userInput, ",")
	patterns := make([]string, 0, len(split))
	for _, pattern := range split {
		p := strings.TrimSpace(pattern)
		if p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// Wait for results and send them to outbox.
// Also wait for user interrupt on inbox.
func iterResults(searchParams *SearchParams, inbox chan Message, outbox chan Message) {
	maxResults := 50
	resultsChan := Search(searchParams)

	for {
		select {
		case result, ok := <-resultsChan.C:
			if !ok {
				// Work is done. Results channel closed.
				outbox <- Message{
					Text: "Все.",
				}
				return
			} else {
				outbox <- Message{
					Text: fmt.Sprintf(
						"%s нашлось в %s.\nftp://free:free@ftp.zakupki.gov.ru%s",
						result.Match,
						result.XmlName,
						result.ZipPath,
					),
					File:     result.XmlFile,
					FileName: result.XmlName,
				}
				maxResults--
				if maxResults == 0 {
					resultsChan.ForceClose()
					return
				}
			}
		case msg := <-inbox:
			if msg.Text == "/stop" {
				outbox <- Message{
					Text: "Отменено.",
				}
				resultsChan.ForceClose()
				return
			}
		}
	}
}
