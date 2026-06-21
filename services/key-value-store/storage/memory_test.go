package storage

import (
	"context"
	"testing"
)

func TestMemoryStore_PutGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	s.Put(ctx, "foo", "bar")
	v, err := s.Get(ctx, "foo")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if v != "bar" {
		t.Errorf("expected bar, got %s", v)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get(context.Background(), "nope")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	s.Put(ctx, "x", "y")
	s.Delete(ctx, "x")
	_, err := s.Get(ctx, "x")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
