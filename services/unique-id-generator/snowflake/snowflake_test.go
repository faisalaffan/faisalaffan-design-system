package snowflake

import (
	"sync"
	"testing"
)

func TestGenerator_Next(t *testing.T) {
	g, err := New(5)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	seen := make(map[int64]bool)
	for i := 0; i < 1000; i++ {
		id, err := g.Next()
		if err != nil {
			t.Fatalf("Next failed: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive ID, got %d", id)
		}
		if seen[id] {
			t.Errorf("duplicate ID: %d", id)
		}
		seen[id] = true
	}
}

func TestGenerator_UniqueAcrossGoroutines(t *testing.T) {
	g, _ := New(1)
	seen := make(map[int64]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				id, _ := g.Next()
				mu.Lock()
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != 1000 {
		t.Errorf("expected 1000 unique IDs, got %d", len(seen))
	}
}

func TestGenerator_InvalidWorker(t *testing.T) {
	_, err := New(-1)
	if err == nil {
		t.Error("expected error for negative worker ID")
	}
	_, err = New(1024)
	if err == nil {
		t.Error("expected error for worker ID > 1023")
	}
}

func TestGenerator_Monotonic(t *testing.T) {
	g, _ := New(0)
	var prev int64
	for i := 0; i < 10000; i++ {
		id, _ := g.Next()
		if id <= prev {
			t.Errorf("IDs not monotonic: %d after %d", id, prev)
		}
		prev = id
	}
}
