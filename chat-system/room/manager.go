package room

import (
	"fmt"
	"sync"
)

type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewManager() *Manager {
	return &Manager{rooms: make(map[string]*Room)}
}

func (m *Manager) GetOrCreate(name string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[name]; ok {
		return r
	}
	r := New(name)
	m.rooms[name] = r
	return r
}

func (m *Manager) Get(name string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rooms[name]
	if !ok {
		return nil, fmt.Errorf("room %s not found", name)
	}
	return r, nil
}

func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.rooms))
	for name := range m.rooms {
		names = append(names, name)
	}
	return names
}
