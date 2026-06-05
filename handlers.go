package main

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleUpdate(bot *tgbotapi.BotAPI, update *tgbotapi.Update, storage Storage, sm *StateManager) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	text := strings.TrimSpace(update.Message.Text)

	if strings.HasPrefix(text, "/") {
		handleCommand(bot, chatID, userID, text, storage, sm)
		return
	}

	if sm.Get(userID) == StateAwaitingNoteText {
		handleAdd(bot, chatID, userID, text, storage, sm)
		return
	}

	handleFreeText(bot, chatID, text)
}

func handleCommand(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, sm *StateManager) {
	parts := strings.SplitN(text, " ", 2)
	command := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	switch command {
	case "/start":
		msg := tgbotapi.NewMessage(chatID, "Привет! Я твой персональный ассистент.\n\n/add <текст> — добавить заметку\n/list — показать все заметки\n/find <запрос> — найти заметки\n/del <номер> — удалить заметку")
		msg.ReplyMarkup = mainMenuKeyboard()
		if _, err := bot.Send(msg); err != nil {
			log.Printf("ошибка отправки сообщения: %v", err)
		}
	case "/help":
		sendMessage(bot, chatID, "Я помогаю сохранять и находить информацию.\n\n*Команды:*\n/add <текст> — сохранить новую запись\n/list — показать все записи\n/find <слово> — поиск по записям\n/help — эта справка")
	case "/add":
		handleAdd(bot, chatID, userID, args, storage, sm)
	case "/list":
		handleList(bot, chatID, userID, storage)
	case "/find":
		handleFind(bot, chatID, userID, args, storage)
	case "/del":
		handleDelete(bot, chatID, userID, args, storage)
	default:
		sendMessage(bot, chatID, "Неизвестная команда. Напиши /help.")
	}
}

func handleAdd(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, sm *StateManager) {
	if text == "" {
		sm.Set(userID, StateAwaitingNoteText)
		sendMessage(bot, chatID, "Отправь мне текст заметки.")
		return
	}

	saveNote(bot, chatID, userID, text, storage, sm)
}

func saveNote(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, sm *StateManager) {
	sm.Clear(userID)

	id, err := storage.Add(userID, text)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при сохранении записи.")
		return
	}

	sendMessage(bot, chatID, fmt.Sprintf("✅ Запись #%d сохранена!", id))
}

func handleList(bot *tgbotapi.BotAPI, chatID int64, userID int64, storage Storage) {
	notes, err := storage.List(userID)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при получении записей.")
		return
	}

	msg := formatNotesList(notes)
	sendMessage(bot, chatID, msg)
}

func handleFind(bot *tgbotapi.BotAPI, chatID int64, userID int64, query string, storage Storage) {
	if query == "" {
		sendMessage(bot, chatID, "Использование: /find <текст>\nПример: /find молоко")
		return
	}

	notes, err := storage.Find(userID, query)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при поиске.")
		return
	}

	msg := formatNotesFound(notes, query)
	sendMessage(bot, chatID, msg)
}

func handleDelete(bot *tgbotapi.BotAPI, chatID int64, userID int64, args string, storage Storage) {
	if args == "" {
		sendMessage(bot, chatID, "Использование: /del <номер>\nНомер можно узнать через /list.\nПример: /del 3")
		return
	}

	var id int
	if _, err := fmt.Sscanf(args, "%d", &id); err != nil {
		sendMessage(bot, chatID, "Укажи номер записи цифрой.\nПример: /del 3")
		return
	}

	if err := storage.Delete(userID, id); err != nil {
		sendMessage(bot, chatID, fmt.Sprintf("Ошибка: %v", err))
		return
	}

	sendMessage(bot, chatID, fmt.Sprintf("🗑 Запись #%d удалена!", id))
}

func handleFreeText(bot *tgbotapi.BotAPI, chatID int64, text string) {
	lower := strings.ToLower(text)

	switch {
	case containsAny(lower, "что ты можешь", "что ты умеешь", "твои возможности", "какие функции"):
		sendMessage(bot, chatID, "Я умею:\n\n📝 /add <текст> — сохранить заметку\n📋 /list — показать все заметки\n🔍 /find <запрос> — найти заметки\n❓ /help — справка по командам")
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func sendMessage(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"

	if _, err := bot.Send(msg); err != nil {
		log.Printf("ошибка отправки сообщения: %v", err)
	}
}
