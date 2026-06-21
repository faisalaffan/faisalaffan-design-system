package algorithm

import (
	"sync"
	"time"
)

type TokenBucket struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

func NewTokenBucket() *TokenBucket {
	return &TokenBucket{buckets: make(map[string]*bucket)}
}

func (tb *TokenBucket) Allow(key string, limit int, window time.Duration) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(limit) - 1, lastRefill: time.Now()}
		tb.buckets[key] = b
		return true
	}

	rate := float64(limit) / window.Seconds()
	elapsed := time.Since(b.lastRefill).Seconds()
	b.tokens += elapsed * rate
	if b.tokens > float64(limit) {
		b.tokens = float64(limit)
	}
	b.lastRefill = time.Now()

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
