package main

import (
	"sync"
)

type chatState struct {
	step  string
	mode  string // create | review | edit
	dirty bool
}

type stateStore struct {
	mu sync.Mutex
	m  map[int64]chatState
}

func newStateStore() *stateStore {
	return &stateStore{m: map[int64]chatState{}}
}

func (s *stateStore) get(chatID int64) chatState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[chatID]
}

func (s *stateStore) step(chatID int64) string {
	return s.get(chatID).step
}

func (s *stateStore) mode(chatID int64) string {
	return s.get(chatID).mode
}

func (s *stateStore) set(chatID int64, step, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.m[chatID]
	st.step = step
	st.mode = mode
	s.m[chatID] = st
}

func (s *stateStore) markDirty(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.m[chatID]
	st.dirty = true
	s.m[chatID] = st
}

func (s *stateStore) dirty(chatID int64) bool {
	return s.get(chatID).dirty
}

func (s *stateStore) clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, chatID)
}
