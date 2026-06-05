package main

import (
	"fmt"
	"strings"
	"sync"
)

type Note struct {
	ID   int
	Text string
}

type Storage interface {
	Add(userID int64, text string) (int, error)
	List(userID int64) ([]Note, error)
	Find(userID int64, query string) ([]Note, error)
	Delete(userID int64, id int) error
}

type MemoryStorage struct {
	mu     sync.RWMutex
	notes  map[int64][]Note
	nextID map[int64]int
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		notes:  make(map[int64][]Note),
		nextID: make(map[int64]int),
	}
}

func (s *MemoryStorage) Add(userID int64, text string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID[userID]
	s.nextID[userID] = id + 1

	note := Note{ID: id, Text: text}
	s.notes[userID] = append(s.notes[userID], note)

	return id, nil
}

func (s *MemoryStorage) List(userID int64) ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notes := s.notes[userID]
	if notes == nil {
		return []Note{}, nil
	}

	result := make([]Note, len(notes))
	copy(result, notes)

	return result, nil
}

func (s *MemoryStorage) Find(userID int64, query string) ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notes := s.notes[userID]
	if notes == nil {
		return []Note{}, nil
	}

	lowerQuery := strings.ToLower(query)
	var result []Note

	for _, note := range notes {
		if strings.Contains(strings.ToLower(note.Text), lowerQuery) {
			result = append(result, note)
		}
	}

	if result == nil {
		return []Note{}, nil
	}

	return result, nil
}

func (s *MemoryStorage) Delete(userID int64, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notes := s.notes[userID]

	for i, note := range notes {
		if note.ID == id {
			s.notes[userID] = append(notes[:i], notes[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("запись #%d не найдена", id)
}

func formatNotesList(notes []Note) string {
	if len(notes) == 0 {
		return "Записей пока нет."
	}

	var sb strings.Builder
	sb.WriteString("📋 *Список записей:*\n\n")

	for _, note := range notes {
		sb.WriteString(fmt.Sprintf("`#%d` %s\n", note.ID, note.Text))
	}

	return sb.String()
}

func formatNotesFound(notes []Note, query string) string {
	if len(notes) == 0 {
		return "Ничего не найдено по запросу: \"" + query + "\"."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Найдено %d записей:*\n\n", len(notes)))

	for _, note := range notes {
		sb.WriteString(fmt.Sprintf("`#%d` %s\n", note.ID, note.Text))
	}

	return sb.String()
}
