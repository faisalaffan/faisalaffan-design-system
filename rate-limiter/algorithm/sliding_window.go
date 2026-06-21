package algorithm

import (
	"sync"
	"time"
)

type slidingEntry struct {
	timestamps []int64
}

type SlidingWindow struct {
	mu      sync.Mutex
	windows map[string]*slidingEntry
}

func NewSlidingWindow() *SlidingWindow {
	return &SlidingWindow{windows: make(map[string]*slidingEntry)}
}

func (sw *SlidingWindow) Allow(key string, limit int, window time.Duration) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now().UnixNano()
	cutoff := now - window.Nanoseconds()

	w, ok := sw.windows[key]
	if !ok {
		sw.windows[key] = &slidingEntry{timestamps: []int64{now}}
		return true
	}

	var valid []int64
	for _, ts := range w.timestamps {
		if ts > cutoff {
			valid = append(valid, ts)
		}
	}

	if len(valid) < limit {
		valid = append(valid, now)
		w.timestamps = valid
		return true
	}

	w.timestamps = valid
	return false
}
