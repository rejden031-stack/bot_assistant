package main

import "sync"

type UserState int

const (
	StateNone UserState = iota
	StateAwaitingNoteText
	StateAwaitingDeleteNumber
)

type StateManager struct {
	mu     sync.Mutex
	states map[int64]UserState
}

func NewStateManager() *StateManager {
	return &StateManager{
		states: make(map[int64]UserState),
	}
}

func (sm *StateManager) Get(userID int64) UserState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.states[userID]
}

func (sm *StateManager) Set(userID int64, state UserState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.states[userID] = state
}

func (sm *StateManager) Clear(userID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.states, userID)
}
