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

func (s *MemoryStore) Get(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *MemoryStore) Put(_ context.Context, key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
