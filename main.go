package main

import (
	"log"
	"net/http"
	"net/url"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	token := getToken()
	bot := createBot(token)

	storage := NewMemoryStorage()
	stateManager := NewStateManager()

	startHealthServer()
	setupCommands(bot)

	log.Printf("Бот запущен как: %s", bot.Self.UserName)

	updates := startUpdates(bot)
	handleUpdates(bot, updates, storage, stateManager)
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
		return token
	}

	token = "PASTE_YOUR_TOKEN_HERE"
	if token == "PASTE_YOUR_TOKEN_HERE" {
		log.Fatal("Токен не указан! Установите переменную окружения BOT_TOKEN или вставьте токен в код.")
	}

	return token
}

func startUpdates(bot *tgbotapi.BotAPI) tgbotapi.UpdatesChannel {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	return bot.GetUpdatesChan(updateConfig)
}

func handleUpdates(bot *tgbotapi.BotAPI, updates tgbotapi.UpdatesChannel, storage Storage, sm *StateManager) {
	for update := range updates {
		handleUpdate(bot, &update, storage, sm)
	}
}

func setupCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{Command: "add", Description: "Добавить заметку"},
		{Command: "list", Description: "Показать все заметки"},
		{Command: "find", Description: "Найти заметки"},
		{Command: "del", Description: "Удалить заметку по номеру"},
		{Command: "clear", Description: "Удалить все заметки"},
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
		{Text: "/clear"},
	}
	row4 := []tgbotapi.KeyboardButton{
		{Text: "/help"},
	}

	return tgbotapi.NewReplyKeyboard(row1, row2, row3, row4)
}
