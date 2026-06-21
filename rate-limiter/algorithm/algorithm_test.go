package algorithm

import (
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/rate-limiter/storage"
)

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket()
	limit := 10
	window := time.Second
	for i := 0; i < limit; i++ {
		if !tb.Allow("test", limit, window) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if tb.Allow("test", limit, window) {
		t.Error("request over limit should be denied")
	}
}

func TestSlidingWindow_Allow(t *testing.T) {
	sw := NewSlidingWindow()
	for i := 0; i < 5; i++ {
		if !sw.Allow("key", 5, time.Second) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if sw.Allow("key", 5, time.Second) {
		t.Error("6th request should be denied")
	}
}

func TestFixedWindow_Allow(t *testing.T) {
	fw := NewFixedWindow()
	for i := 0; i < 3; i++ {
		if !fw.Allow("x", 3, time.Second) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if fw.Allow("x", 3, time.Second) {
		t.Error("request over limit denied")
	}
}

func TestMemoryStore_Increment(t *testing.T) {
	s := storage.NewMemoryStore()
	count, _ := s.Increment("a", time.Minute)
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
	count, _ = s.Increment("a", time.Minute)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestMemoryStore_Count(t *testing.T) {
	s := storage.NewMemoryStore()
	s.Increment("b", time.Minute)
	s.Increment("b", time.Minute)
	count, _ := s.Count("b", time.Minute)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestMemoryStore_Reset(t *testing.T) {
	s := storage.NewMemoryStore()
	s.Increment("c", time.Minute)
	s.Reset("c")
	count, _ := s.Count("c", time.Minute)
	if count != 0 {
		t.Errorf("expected 0 after reset, got %d", count)
	}
}

func TestAlgorithm_Interface(t *testing.T) {
	// Compile-time check: all types satisfy Algorithm interface
	var _ Algorithm = NewTokenBucket()
	var _ Algorithm = NewSlidingWindow()
	var _ Algorithm = NewFixedWindow()
}
