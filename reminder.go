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
var reminderLocation = time.UTC
var pendingReminderTimes = make(map[int64]time.Time)
var pendingMu sync.Mutex

func setPendingTime(userID int64, t time.Time) {
	pendingMu.Lock()
	pendingReminderTimes[userID] = t
	pendingMu.Unlock()
}

func getPendingTime(userID int64) (time.Time, bool) {
	pendingMu.Lock()
	t, ok := pendingReminderTimes[userID]
	delete(pendingReminderTimes, userID)
	pendingMu.Unlock()
	return t, ok
}

func initReminderLocation() {
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("reminder timezone %s: %v, using UTC", tz, err)
		return
	}
	reminderLocation = loc
	log.Printf("Напоминания: часовой пояс %s", tz)
}

func nowLocal() time.Time {
	return time.Now().In(reminderLocation)
}

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
	s := strings.TrimSpace(input)
	if s == "" {
		return time.Time{}, "", fmt.Errorf("напиши так: в 19:00 <текст>")
	}

	if t, text, err := tryParseThrough(s); err == nil {
		return t, text, nil
	}

	if t, text, err := tryParseTomorrow(s); err == nil {
		return t, text, nil
	}

	return tryParseTime(s)
}

func tryParseThrough(s string) (time.Time, string, error) {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "через ")
	if idx < 0 {
		return time.Time{}, "", fmt.Errorf("не найдено")
	}

	after := strings.TrimSpace(s[idx+6:])
	parts := strings.SplitN(after, " ", 2)
	if len(parts) < 2 {
		return time.Time{}, "", fmt.Errorf("напиши: через N минут/часов <текст>")
	}

	num, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял число: %s", parts[0])
	}

	unitParts := strings.SplitN(parts[1], " ", 2)
	unit := strings.ToLower(unitParts[0])
	text := ""
	if len(unitParts) > 1 {
		text = unitParts[1]
	}

	dur := 0
	switch {
	case strings.HasPrefix(unit, "мин"):
		dur = num
	case strings.HasPrefix(unit, "ч"):
		dur = num * 60
	default:
		return time.Time{}, "", fmt.Errorf("поддерживаю минуты и часы. Пример: через 30 минут")
	}

	prefixText := strings.TrimSpace(s[:idx])
	if prefixText != "" {
		text = prefixText + " " + text
	}

	return nowLocal().Add(time.Duration(dur) * time.Minute), strings.TrimSpace(text), nil
}

func tryParseTomorrow(s string) (time.Time, string, error) {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "завтра")
	if idx < 0 {
		return time.Time{}, "", fmt.Errorf("не найдено")
	}

	after := strings.TrimSpace(s[idx+6:])
	after = strings.TrimSpace(strings.TrimPrefix(after, "в "))

	parts := strings.SplitN(after, " ", 2)
	if len(parts) < 1 || parts[0] == "" {
		return time.Time{}, "", fmt.Errorf("напиши: завтра в 19:00 <текст>")
	}

	t, err := time.Parse("15:04", parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял время. Пиши: завтра в 19:00 <текст>")
	}

	now := nowLocal()
	text := ""
	if len(parts) > 1 {
		text = parts[1]
	}

	prefixText := strings.TrimSpace(s[:idx])
	if prefixText != "" {
		text = prefixText + " " + text
	}

	result := time.Date(now.Year(), now.Month(), now.Day()+1, t.Hour(), t.Minute(), 0, 0, reminderLocation)
	return result, strings.TrimSpace(text), nil
}

func tryParseTime(s string) (time.Time, string, error) {
	lower := strings.ToLower(s)
	idx := strings.LastIndex(lower, "в ")
	if idx < 0 {
		return time.Time{}, "", fmt.Errorf("не нашёл время. Пиши: в 19:00 <текст> или через 30 минут <текст>")
	}

	after := strings.TrimSpace(s[idx+2:])
	parts := strings.SplitN(after, " ", 2)

	t, err := time.Parse("15:04", parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("не понял время. Пиши: в 19:00 <текст> или через 30 минут <текст>")
	}

	now := nowLocal()
	text := ""
	if len(parts) > 1 {
		text = parts[1]
	}

	prefixText := strings.TrimSpace(s[:idx])
	if prefixText != "" {
		text = prefixText + " " + text
	}

	result := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, reminderLocation)
	if result.Before(now) {
		result = result.Add(24 * time.Hour)
	}

	return result, strings.TrimSpace(text), nil
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
