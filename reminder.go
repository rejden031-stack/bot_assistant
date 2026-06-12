package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Reminder struct {
	ID     int       `json:"id"`
	UserID int64     `json:"user_id"`
	ChatID int64     `json:"chat_id"`
	Time   time.Time `json:"time"`
	Text   string    `json:"text"`
	Sent   bool      `json:"sent"`
}

type ReminderStore struct {
	mu        sync.RWMutex
	reminders []Reminder
	nextID    int
	filePath  string
}

var reminderStore *ReminderStore

func NewReminderStore() *ReminderStore {
	dir := "data"
	os.MkdirAll(dir, 0755)

	store := &ReminderStore{
		filePath: filepath.Join(dir, "reminders.json"),
		nextID:   1,
	}
	store.load()
	return store
}

func (s *ReminderStore) Add(userID, chatID int64, t time.Time, text string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++

	s.reminders = append(s.reminders, Reminder{
		ID:     id,
		UserID: userID,
		ChatID: chatID,
		Time:   t,
		Text:   text,
	})

	s.save()
	return id
}

func (s *ReminderStore) ListPending(userID int64) []Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Reminder
	for _, r := range s.reminders {
		if r.UserID == userID && !r.Sent {
			result = append(result, r)
		}
	}
	return result
}

func (s *ReminderStore) ListAll(userID int64) []Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Reminder
	for _, r := range s.reminders {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result
}

func (s *ReminderStore) Delete(userID int64, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.reminders {
		if r.ID == id && r.UserID == userID {
			s.reminders = append(s.reminders[:i], s.reminders[i+1:]...)
			s.save()
			return nil
		}
	}
	return fmt.Errorf("напоминание #%d не найдено", id)
}

func (s *ReminderStore) checkAndSend(bot *tgbotapi.BotAPI) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for i := range s.reminders {
		if s.reminders[i].Sent {
			continue
		}
		if now.After(s.reminders[i].Time) || now.Equal(s.reminders[i].Time) {
			s.reminders[i].Sent = true
			msg := tgbotapi.NewMessage(s.reminders[i].ChatID,
				fmt.Sprintf("⏰ *Напоминание:* %s", s.reminders[i].Text))
			if _, err := bot.Send(msg); err != nil {
				log.Printf("reminder send error: %v", err)
			}
		}
	}
	s.save()
}

func (s *ReminderStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.reminders)
	for _, r := range s.reminders {
		if r.ID >= s.nextID {
			s.nextID = r.ID + 1
		}
	}
}

func (s *ReminderStore) save() {
	data, _ := json.MarshalIndent(s.reminders, "", "  ")
	os.WriteFile(s.filePath, data, 0644)
}

func parseReminderText(input string) (time.Time, string, error) {
	lower := strings.ToLower(strings.TrimSpace(input))

	if strings.HasPrefix(lower, "через ") {
		return parseThrough(strings.TrimSpace(input[6:]))
	}

	if strings.HasPrefix(lower, "завтра ") {
		return parseTomorrow(strings.TrimSpace(input[7:]))
	}

	input = strings.TrimSpace(strings.TrimPrefix(input, "в "))
	return parseToday(input)
}

func parseThrough(s string) (time.Time, string, error) {
	parts := strings.SplitN(s, " ", 2)
	if len(parts) < 2 {
		return time.Time{}, "", fmt.Errorf("напиши так: через 30 минут <текст>")
	}

	num, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял число: %s", parts[0])
	}

	text := ""
	dur := 0
	unit := strings.ToLower(parts[1])

	switch {
	case strings.HasPrefix(unit, "мин"):
		dur = num
	case strings.HasPrefix(unit, "ч"):
		dur = num * 60
	default:
		return time.Time{}, "", fmt.Errorf("поддерживаю только минуты и часы. Пример: через 30 минут <текст>")
	}

	rest := strings.TrimSpace(s[len(parts[0])+len(unit):])
	text = strings.TrimSpace(strings.TrimPrefix(rest, unit))

	return time.Now().Add(time.Duration(dur) * time.Minute), text, nil
}

func parseTomorrow(s string) (time.Time, string, error) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "в "))
	parts := strings.SplitN(s, " ", 2)
	if len(parts) < 1 {
		return time.Time{}, "", fmt.Errorf("напиши так: завтра в 19:00 <текст>")
	}

	t, err := time.Parse("15:04", parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял время. Пиши: завтра в 19:00 <текст>")
	}

	now := time.Now()
	text := ""
	if len(parts) > 1 {
		text = parts[1]
	}

	result := time.Date(now.Year(), now.Month(), now.Day()+1, t.Hour(), t.Minute(), 0, 0, now.Location())
	return result, text, nil
}

func parseToday(s string) (time.Time, string, error) {
	parts := strings.SplitN(s, " ", 2)
	if len(parts) < 1 {
		return time.Time{}, "", fmt.Errorf("напиши так: в 19:00 <текст>")
	}

	t, err := time.Parse("15:04", parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял время. Пиши: в 19:00 <текст> или через 30 минут <текст>")
	}

	now := time.Now()
	text := ""
	if len(parts) > 1 {
		text = parts[1]
	}

	result := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if result.Before(now) {
		result = result.Add(24 * time.Hour)
	}

	return result, text, nil
}

func startReminderScheduler(bot *tgbotapi.BotAPI) {
	if reminderStore == nil {
		return
	}
	go func() {
		for {
			time.Sleep(30 * time.Second)
			reminderStore.checkAndSend(bot)
		}
	}()
}
