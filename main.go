package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	token := getToken()
	bot := createBot(token)

	storage := NewMemoryStorage()
	stateManager := NewStateManager()
	ai := NewAIClient(os.Getenv("OPENROUTER_API_KEY"))

	startHealthServer()
	setupCommands(bot)

	if ai != nil {
		log.Printf("AI (OpenRouter) подключён")
	} else {
		log.Printf("AI не подключён — установите OPENROUTER_API_KEY")
	}

	reminderStore = NewReminderStore()
	startReminderScheduler(bot)
	log.Printf("Напоминания активны")

	log.Printf("Бот запущен как: %s", bot.Self.UserName)

	updates := startUpdates(bot)
	handleUpdates(bot, updates, storage, stateManager, ai)
}

func startHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	go func() {
		log.Printf("Health server listening on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Health server stopped: %v", err)
		}
	}()

	go selfPing()
}

func selfPing() {
	url := os.Getenv("RENDER_EXTERNAL_URL")
	if url == "" {
		url = "https://bot-assistant-vhbf.onrender.com"
	}
	healthURL := url + "/health"

	for {
		time.Sleep(4 * time.Minute)
		resp, err := http.Get(healthURL)
		if err != nil {
			log.Printf("self-ping error: %v", err)
			continue
		}
		resp.Body.Close()
		log.Printf("self-ping: %s", resp.Status)
	}
}

func createBot(token string) *tgbotapi.BotAPI {
	proxyURL := os.Getenv("TG_PROXY")

	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			log.Fatalf("неверный формат TG_PROXY: %v", err)
		}

		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(u),
			},
		}

		bot, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, client)
		if err != nil {
			log.Fatalf("не удалось создать бота (через прокси): %v", err)
		}

		return bot
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("не удалось создать бота: %v", err)
	}

	return bot
}

func getToken() string {
	token := os.Getenv("BOT_TOKEN")
	if token != "" {
		if len(token) > 4 {
			log.Printf("Токен из BOT_TOKEN (…%s)", token[len(token)-4:])
		}
		return token
	}

	filePath := os.Getenv("BOT_TOKEN_FILE")
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Не удалось прочитать BOT_TOKEN_FILE (%s): %v", filePath, err)
		}
		token = strings.TrimSpace(string(data))
		if token == "" {
			log.Fatal("BOT_TOKEN_FILE пуст")
		}
		if len(token) > 4 {
			log.Printf("Токен из BOT_TOKEN_FILE (…%s)", token[len(token)-4:])
		}
		return token
	}

	log.Fatal("Токен не указан! Установите BOT_TOKEN или BOT_TOKEN_FILE.")
	return ""
}

func startUpdates(bot *tgbotapi.BotAPI) tgbotapi.UpdatesChannel {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	return bot.GetUpdatesChan(updateConfig)
}

func handleUpdates(bot *tgbotapi.BotAPI, updates tgbotapi.UpdatesChannel, storage Storage, sm *StateManager, ai *AIClient) {
	for update := range updates {
		handleUpdate(bot, &update, storage, sm, ai)
	}
}

func setupCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{Command: "add", Description: "Добавить заметку"},
		{Command: "list", Description: "Показать все заметки"},
		{Command: "find", Description: "Найти заметки"},
		{Command: "del", Description: "Удалить заметку по номеру"},
		{Command: "clear", Description: "Удалить все заметки"},
		{Command: "ask", Description: "Спросить AI-ассистента"},
		{Command: "remind", Description: "Создать напоминание"},
		{Command: "reminders", Description: "Список напоминаний"},
		{Command: "reminddel", Description: "Удалить напоминание"},
		{Command: "help", Description: "Справка"},
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)

	if _, err := bot.Request(cfg); err != nil {
		log.Printf("не удалось установить меню команд: %v", err)
	}
}

func mainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	row1 := []tgbotapi.KeyboardButton{
		{Text: "/add"},
		{Text: "/list"},
	}
	row2 := []tgbotapi.KeyboardButton{
		{Text: "/find"},
		{Text: "/del"},
	}
	row3 := []tgbotapi.KeyboardButton{
		{Text: "/ask"},
		{Text: "/remind"},
	}
	row4 := []tgbotapi.KeyboardButton{
		{Text: "/reminders"},
		{Text: "/help"},
	}

	return tgbotapi.NewReplyKeyboard(row1, row2, row3, row4)
}
