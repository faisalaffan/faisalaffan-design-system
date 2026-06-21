package store

import (
	"testing"
	"time"
)

func TestMemoryStore_Create(t *testing.T) {
	s := NewMemoryStore()
	v := s.Create("Title", "Description", []string{"tag1"})

	if v.ID == "" {
		t.Error("expected non-empty ID")
	}
	if v.Title != "Title" {
		t.Errorf("expected Title, got %s", v.Title)
	}
	if v.Status != "uploading" {
		t.Errorf("expected uploading, got %s", v.Status)
	}

	// Wait for simulated transcoding
	time.Sleep(1500 * time.Millisecond)

	v2 := s.Get(v.ID)
	if v2.Status != "ready" {
		t.Errorf("expected ready after transcode, got %s", v2.Status)
	}
}

func TestMemoryStore_Search(t *testing.T) {
	s := NewMemoryStore()
	s.Create("Go Programming", "Learn Go", []string{"coding"})
	s.Create("Python Guide", "Learn Python", []string{"coding"})

	results := s.Search("go")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'go', got %d", len(results))
	}

	results = s.Search("coding")
	if len(results) != 2 {
		t.Errorf("expected 2 results for tag 'coding', got %d", len(results))
	}
}

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	s.Create("A", "", nil)
	s.Create("B", "", nil)

	videos := s.List()
	if len(videos) != 2 {
		t.Errorf("expected 2 videos, got %d", len(videos))
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryStore()
	v := s.Get("noexist")
	if v != nil {
		t.Error("expected nil for missing video")
	}
}
