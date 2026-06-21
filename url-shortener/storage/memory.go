package storage

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

func (s *MemoryStore) Save(_ context.Context, code string, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[code] = url
	return nil
}

func (s *MemoryStore) Get(_ context.Context, code string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	url, ok := s.data[code]
	if !ok {
		return "", ErrNotFound
	}
	return url, nil
}

func (s *MemoryStore) Exists(_ context.Context, code string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[code]
	return ok, nil
}
