package storage

import (
	"context"
	"testing"
)

func TestMemoryStore_SaveGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	if err := s.Save(ctx, "abc1234", "https://example.com"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	url, err := s.Get(ctx, "abc1234")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("expected https://example.com, got %s", url)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get(context.Background(), "nope")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Exists(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	s.Save(ctx, "key1", "url1")

	ok, _ := s.Exists(ctx, "key1")
	if !ok {
		t.Error("expected key1 to exist")
	}

	ok, _ = s.Exists(ctx, "nope")
	if ok {
		t.Error("expected nope to not exist")
	}
}
