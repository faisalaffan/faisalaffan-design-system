package storage

import (
	"sync"
	"time"
)

type entry struct {
	count   int
	resetAt time.Time
}

type MemoryStore struct {
	mu   sync.Mutex
	data map[string]*entry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*entry)}
}

func (s *MemoryStore) Increment(key string, window time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || time.Now().After(e.resetAt) {
		s.data[key] = &entry{count: 1, resetAt: time.Now().Add(window)}
		return 1, nil
	}
	e.count++
	return e.count, nil
}

func (s *MemoryStore) Count(key string, window time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || time.Now().After(e.resetAt) {
		return 0, nil
	}
	return e.count, nil
}

func (s *MemoryStore) Reset(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
