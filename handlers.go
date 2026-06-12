package main

import (
	"fmt"
	"log"
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
		sendMessage(bot, chatID, "Голосовые сообщения не поддерживаются.")
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
	case StateAwaitingFindQuery:
		handleFindQuery(bot, chatID, userID, text, storage, sm)
		return
	case StateAwaitingRemindText:
		handleRemindText(bot, chatID, userID, text, sm)
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
		msg := tgbotapi.NewMessage(chatID, "Привет! Я твой персональный ассистент.\n\n/add <текст> — добавить заметку\n/list — показать все заметки\n/find <запрос> — найти заметки\n/del <номер> — удалить заметку\n/clear — удалить все заметки\n/ask <вопрос> — спросить AI\n/remind — создать напоминание\n/reminders — список напоминаний\n\nТакже можно просто написать текст — AI ответит.")
		msg.ReplyMarkup = mainMenuKeyboard()
		if _, err := bot.Send(msg); err != nil {
			log.Printf("ошибка отправки сообщения: %v", err)
		}
	case "/help":
		sendMessage(bot, chatID, "Я помогаю сохранять и находить информацию.\n\n*Команды:*\n/add <текст> — сохранить новую запись\n/list — показать все записи\n/find <слово> — поиск по записям\n/del <номер> — удалить запись\n/clear — удалить все записи\n/ask <вопрос> — спросить AI\n/remind — создать напоминание\n/reminders — список напоминаний\n/reminddel <номер> — удалить напоминание\n/help — эта справка")
	case "/add":
		handleAdd(bot, chatID, userID, args, storage, sm)
	case "/list":
		handleList(bot, chatID, userID, storage)
	case "/find":
		if args == "" {
			sm.Set(userID, StateAwaitingFindQuery)
			sendMessage(bot, chatID, "Что ищем?")
			return
		}
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
			sendMessage(bot, chatID, "AI не подключён. Установите OPENROUTER_API_KEY.")
			return
		}
		if args == "" {
			sendMessage(bot, chatID, "Напиши вопрос после /ask.\nПример: /ask какая погода в Москве?")
			return
		}
		handleAskAI(bot, chatID, userID, args, storage, ai)
	case "/remind":
		if args == "" {
			sm.Set(userID, StateAwaitingRemindText)
			sendMessage(bot, chatID, "Во сколько и что напомнить?\nНапример: в 19:00 накормить кота")
			return
		}
		handleRemind(bot, chatID, userID, args, sm)
	case "/reminders":
		handleReminders(bot, chatID, userID)
	case "/reminddel":
		if args == "" {
			sendMessage(bot, chatID, "Использование: /reminddel <номер>\nСмотри /reminders для списка номеров.")
			return
		}
		handleRemindDel(bot, chatID, userID, args)
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
	trimmed := strings.TrimSpace(text)

	switch {
	case containsAny(lower, "что ты можешь", "что ты умеешь", "твои возможности", "какие функции"):
		sendMessage(bot, chatID, "Я умею:\n\n📝 /add <текст> — сохранить заметку\n📋 /list — показать все заметки\n🔍 /find <запрос> — найти заметки\n/del — удалить заметку\n/clear — удалить все\n/ask <вопрос> — спросить AI\n/remind — создать напоминание\n❓ /help — справка по командам")
		return
	}

	if tryCreateReminder(bot, chatID, userID, lower, trimmed) {
		return
	}

	if ai != nil {
		sendTyping(bot, chatID)
		notes, _ := storage.List(userID)
		answer, err := ai.Ask(text, notes)
		if err != nil {
			sendMessage(bot, chatID, "Ошибка AI: "+err.Error())
			return
		}
		sendPlain(bot, chatID, answer)
		return
	}

	sendMessage(bot, chatID, "Напиши /help чтобы узнать команды. Или добавь OPENROUTER_API_KEY для AI.")
}

func tryCreateReminder(bot *tgbotapi.BotAPI, chatID int64, userID int64, lower, trimmed string) bool {
	prefixes := []string{"напомни ", "напомнить ", "напомнишь "}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			args := trimmed[len(p):]
			return createReminderFromText(bot, chatID, userID, args)
		}
	}
	if strings.HasPrefix(lower, "через ") {
		return createReminderFromText(bot, chatID, userID, trimmed)
	}
	return false
}

func createReminderFromText(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string) bool {
	if reminderStore == nil {
		sendMessage(bot, chatID, "Напоминания временно недоступны.")
		return true
	}
	t, reminderText, err := parseReminderText(text)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка: "+err.Error())
		return true
	}
	if reminderText == "" {
		sendMessage(bot, chatID, "А что напомнить?")
		return true
	}
	id := reminderStore.Add(userID, chatID, t, reminderText)
	sendMessage(bot, chatID, fmt.Sprintf("✅ Напоминание #%d на %s", id, t.Format("2 Jan 15:04")))
	return true
}

func handleFindQuery(bot *tgbotapi.BotAPI, chatID int64, userID int64, query string, storage Storage, sm *StateManager) {
	sm.Clear(userID)
	handleFind(bot, chatID, userID, query, storage)
}

func handleRemind(bot *tgbotapi.BotAPI, chatID int64, userID int64, args string, sm *StateManager) {
	if reminderStore == nil {
		sendMessage(bot, chatID, "Напоминания временно недоступны.")
		return
	}

	t, text, err := parseReminderText(args)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка: "+err.Error())
		return
	}
	if text == "" {
		sm.Set(userID, StateAwaitingRemindText)
		sendMessage(bot, chatID, fmt.Sprintf("Напомню в %s. А что напомнить?", t.Format("15:04")))
		return
	}

	id := reminderStore.Add(userID, chatID, t, text)
	sendMessage(bot, chatID, fmt.Sprintf("✅ Напоминание #%d в %s", id, t.Format("2 Jan 15:04")))
}

func handleRemindText(bot *tgbotapi.BotAPI, chatID int64, userID int64, text string, sm *StateManager) {
	if reminderStore == nil {
		sendMessage(bot, chatID, "Напоминания временно недоступны.")
		return
	}

	t, reminderText, err := parseReminderText(text)
	if err != nil {
		sendMessage(bot, chatID, "Ошибка: "+err.Error())
		return
	}
	if reminderText == "" {
		sendMessage(bot, chatID, "А что напомнить? Напиши что-нибудь ещё.")
		return
	}

	id := reminderStore.Add(userID, chatID, t, reminderText)
	sm.Clear(userID)
	sendMessage(bot, chatID, fmt.Sprintf("✅ Напоминание #%d на %s", id, t.Format("2 Jan 15:04")))
}

func handleReminders(bot *tgbotapi.BotAPI, chatID int64, userID int64) {
	if reminderStore == nil {
		sendMessage(bot, chatID, "Напоминания временно недоступны.")
		return
	}

	pending := reminderStore.ListPending(userID)
	if len(pending) == 0 {
		sendMessage(bot, chatID, "Нет активных напоминаний.")
		return
	}

	msg := "⏰ *Активные напоминания:*\n\n"
	for _, r := range pending {
		msg += fmt.Sprintf("`#%d` %s — ⏱ %s\n", r.ID, r.Text, r.Time.Format("2 Jan 15:04"))
	}
	sendMessage(bot, chatID, msg)
}

func handleRemindDel(bot *tgbotapi.BotAPI, chatID int64, userID int64, args string) {
	if reminderStore == nil {
		sendMessage(bot, chatID, "Напоминания временно недоступны.")
		return
	}

	var id int
	if _, err := fmt.Sscanf(args, "%d", &id); err != nil {
		sendMessage(bot, chatID, "Нужен номер напоминания. Смотри /reminders")
		return
	}

	if err := reminderStore.Delete(userID, id); err != nil {
		sendMessage(bot, chatID, fmt.Sprintf("Ошибка: %v", err))
		return
	}

	sendMessage(bot, chatID, fmt.Sprintf("🗑 Напоминание #%d удалено!", id))
}

func handleAskAI(bot *tgbotapi.BotAPI, chatID int64, userID int64, prompt string, storage Storage, ai *AIClient) {
	sendTyping(bot, chatID)
	notes, _ := storage.List(userID)
	answer, err := ai.Ask(prompt, notes)
	if err != nil {
		sendPlain(bot, chatID, "Ошибка AI: "+err.Error())
		return
	}
	sendPlain(bot, chatID, answer)
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

func sendPlain(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)

	if _, err := bot.Send(msg); err != nil {
		log.Printf("ошибка отправки сообщения: %v", err)
	}
}
