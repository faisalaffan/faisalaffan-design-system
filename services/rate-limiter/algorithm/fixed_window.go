package algorithm

import (
	"sync"
	"time"
)

type fixedEntry struct {
	count  int
	window int64
}

type FixedWindow struct {
	mu      sync.Mutex
	windows map[string]*fixedEntry
}

func NewFixedWindow() *FixedWindow {
	return &FixedWindow{windows: make(map[string]*fixedEntry)}
}

func (fw *FixedWindow) Allow(key string, limit int, window time.Duration) bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now().UnixNano()
	windowNano := window.Nanoseconds()
	currentWindow := now / windowNano * windowNano

	w, ok := fw.windows[key]
	if !ok || w.window != currentWindow {
		fw.windows[key] = &fixedEntry{count: 1, window: currentWindow}
		return true
	}

	if w.count < limit {
		w.count++
		return true
	}
	return false
}
