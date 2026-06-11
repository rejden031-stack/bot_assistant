package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleUpdate(bot *tgbotapi.BotAPI, update *tgbotapi.Update, storage Storage, sm *StateManager, ai *AIClient) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	if update.Message.Voice != nil {
		handleVoice(bot, chatID, userID, update.Message.Voice, storage, sm, ai)
		return
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return
	}

	if strings.HasPrefix(text, "/") {
		handleCommand(bot, chatID, userID, text, storage, sm, ai)
		return
	}

	switch sm.Get(userID) {
	case StateAwaitingNoteText:
		handleAdd(bot, chatID, userID, text, storage, sm)
		return
	case StateAwaitingDeleteNumber:
		handleDeleteNumber(bot, chatID, userID, text, storage, sm)
		return
	}

	handleFreeText(bot, chatID, userID, text, storage, ai)
}

func handleCommand(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, sm *StateManager, ai *AIClient) {
	parts := strings.SplitN(text, " ", 2)
	command := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	switch command {
	case "/start":
		msg := tgbotapi.NewMessage(chatID, "Привет! Я твой персональный ассистент.\n\n/add <текст> — добавить заметку\n/list — показать все заметки\n/find <запрос> — найти заметки\n/del <номер> — удалить заметку\n/clear — удалить все заметки\n/ask <вопрос> — спросить AI\n\nТакже можно просто написать текст или отправить голосовое — AI ответит.")
		msg.ReplyMarkup = mainMenuKeyboard()
		if _, err := bot.Send(msg); err != nil {
			log.Printf("ошибка отправки сообщения: %v", err)
		}
	case "/help":
		sendMessage(bot, chatID, "Я помогаю сохранять и находить информацию.\n\n*Команды:*\n/add <текст> — сохранить новую запись\n/list — показать все записи\n/find <слово> — поиск по записям\n/del <номер> — удалить запись\n/clear — удалить все записи\n/ask <вопрос> — спросить AI\n/help — эта справка")
	case "/add":
		handleAdd(bot, chatID, userID, args, storage, sm)
	case "/list":
		handleList(bot, chatID, userID, storage)
	case "/find":
		handleFind(bot, chatID, userID, args, storage)
	case "/del":
		if args == "" {
			sm.Set(userID, StateAwaitingDeleteNumber)
			sendMessage(bot, chatID, "Введи номер заметки для удаления:")
			return
		}
		handleDeleteNumber(bot, chatID, userID, args, storage, sm)
	case "/clear":
		handleClear(bot, chatID, userID, storage)
	case "/ask":
		if ai == nil {
			sendMessage(bot, chatID, "AI не подключён. Установите GEMINI_API_KEY.")
			return
		}
		if args == "" {
			sendMessage(bot, chatID, "Напиши вопрос после /ask.\nПример: /ask какая погода в Москве?")
			return
		}
		handleAskAI(bot, chatID, userID, args, storage, ai)
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

func handleDeleteNumber(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, sm *StateManager) {
	sm.Clear(userID)

	var id int
	if _, err := fmt.Sscanf(text, "%d", &id); err != nil {
		sendMessage(bot, chatID, "Нужно ввести число. Попробуй ещё раз через /del")
		return
	}

	if err := storage.Delete(userID, id); err != nil {
		sendMessage(bot, chatID, fmt.Sprintf("Ошибка: %v", err))
		return
	}

	sendMessage(bot, chatID, fmt.Sprintf("🗑 Запись #%d удалена!", id))
}

func handleClear(bot *tgbotapi.BotAPI, chatID int64, userID int64, storage Storage) {
	if err := storage.DeleteAll(userID); err != nil {
		sendMessage(bot, chatID, "Ошибка при удалении записей.")
		return
	}
	sendMessage(bot, chatID, "🗑 Все записи удалены!")
}

func handleFreeText(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, storage Storage, ai *AIClient) {
	lower := strings.ToLower(text)

	switch {
	case containsAny(lower, "что ты можешь", "что ты умеешь", "твои возможности", "какие функции"):
		sendMessage(bot, chatID, "Я умею:\n\n📝 /add <текст> — сохранить заметку\n📋 /list — показать все заметки\n🔍 /find <запрос> — найти заметки\n/del — удалить заметку\n/clear — удалить все\n/ask <вопрос> — спросить AI\n❓ /help — справка по командам")
		return
	}

	if ai != nil {
		sendTyping(bot, chatID)
		notes, _ := storage.List(userID)
		answer, err := ai.ask(text, notes, "")
		if err != nil {
			sendMessage(bot, chatID, "Ошибка AI: "+err.Error())
			return
		}
		sendMessage(bot, chatID, answer)
		return
	}

	sendMessage(bot, chatID, "Напиши /help чтобы узнать команды. Или добавь GEMINI_API_KEY для AI.")
}

func handleAskAI(bot *tgbotapi.BotAPI, chatID int64, userID int64, prompt string, storage Storage, ai *AIClient) {
	sendTyping(bot, chatID)
	notes, _ := storage.List(userID)
	answer, err := ai.ask(prompt, notes, "Отвечай развёрнуто и полезно.")
	if err != nil {
		sendMessage(bot, chatID, "Ошибка AI: "+err.Error())
		return
	}
	sendMessage(bot, chatID, answer)
}

func handleVoice(bot *tgbotapi.BotAPI, chatID int64, userID int64, voice *tgbotapi.Voice, storage Storage, sm *StateManager, ai *AIClient) {
	if ai == nil {
		sendMessage(bot, chatID, "AI не подключён. Голосовые сообщения не поддерживаются.")
		return
	}

	sendMessage(bot, chatID, "🎤 Расшифровываю голосовое...")

	file, err := bot.GetFile(tgbotapi.FileConfig{FileID: voice.FileID})
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при получении файла.")
		return
	}

	url := file.Link(bot.Token)
	resp, err := http.Get(url)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при скачивании аудио.")
		return
	}
	defer resp.Body.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка при чтении аудио.")
		return
	}

	sendTyping(bot, chatID)
	text, err := ai.transcribe(audioData)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка распознавания: "+err.Error())
		return
	}

	sendMessage(bot, chatID, "📝 *Распознано:*\n"+text)

	notes, _ := storage.List(userID)
	answer, err := ai.ask(text, notes, "Пользователь отправил голосовое сообщение. Ответь на него.")
	if err != nil {
		return
	}
	sendMessage(bot, chatID, answer)
}

func sendTyping(bot *tgbotapi.BotAPI, chatID int64) {
	bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
	time.Sleep(500 * time.Millisecond)
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
