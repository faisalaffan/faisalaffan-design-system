package store

import (
	"testing"
)

func TestMemoryStore_CreateUser(t *testing.T) {
	s := NewMemoryStore()
	u := s.CreateUser("alice")
	if u.ID != "alice" {
		t.Errorf("expected alice, got %s", u.ID)
	}
}

func TestMemoryStore_CreatePost_FanOut(t *testing.T) {
	s := NewMemoryStore()
	s.CreateUser("a")
	s.CreateUser("b")
	s.Follow("a", "b")

	s.CreatePost("b", "b's post")

	tl := s.GetTimeline("a", 0, 10)
	if len(tl) != 1 {
		t.Errorf("expected 1 post in a's timeline (fan-out from b), got %d", len(tl))
	}
}

func TestMemoryStore_Follow_InvalidUsers(t *testing.T) {
	s := NewMemoryStore()
	s.CreateUser("a")
	if s.Follow("a", "nonexist") {
		t.Error("expected follow to fail for non-existent followee")
	}
	if s.Follow("nonexist", "a") {
		t.Error("expected follow to fail for non-existent follower")
	}
}

func TestMemoryStore_TimelinePagination(t *testing.T) {
	s := NewMemoryStore()
	s.CreateUser("x")
	for i := 0; i < 5; i++ {
		s.CreatePost("x", "post")
	}

	tl := s.GetTimeline("x", 0, 3)
	if len(tl) != 3 {
		t.Errorf("expected 3 posts (limit), got %d", len(tl))
	}

	tl = s.GetTimeline("x", 3, 3)
	if len(tl) != 2 {
		t.Errorf("expected 2 posts (offset 3), got %d", len(tl))
	}
}
