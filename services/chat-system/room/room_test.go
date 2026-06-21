package room

import (
	"testing"
)

func TestManager_GetOrCreate(t *testing.T) {
	mgr := NewManager()
	r1 := mgr.GetOrCreate("general")
	r2 := mgr.GetOrCreate("general")
	if r1 != r2 {
		t.Error("expected same room instance")
	}
}

func TestManager_List(t *testing.T) {
	mgr := NewManager()
	mgr.GetOrCreate("room-a")
	mgr.GetOrCreate("room-b")
	names := mgr.List()
	if len(names) != 2 {
		t.Errorf("expected 2 rooms, got %d", len(names))
	}
}

func TestManager_GetNotFound(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.Get("nope")
	if err == nil {
		t.Error("expected error for missing room")
	}
}

func TestRoom_SendHistory(t *testing.T) {
	r := New("test")
	r.Send(Message{User: "a", Room: "test", Content: "hello"})
	r.Send(Message{User: "b", Room: "test", Content: "world"})

	// Wait for messages to be processed by the room goroutine
	var history []Message
	for len(history) < 2 {
		history = r.History()
	}
	if history[0].Content != "hello" {
		t.Errorf("expected 'hello', got %s", history[0].Content)
	}
}

func TestRoom_ClientCount(t *testing.T) {
	r := New("test")
	if r.ClientCount() != 0 {
		t.Error("expected 0 clients")
	}

	c := &Client{User: "testuser", Send: make(chan []byte, 1)}
	r.Join(c)

	// Give goroutine time to process
	for r.ClientCount() == 0 {
	}
	if r.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", r.ClientCount())
	}

	r.Leave(c)
	for r.ClientCount() == 1 {
	}
	if r.ClientCount() != 0 {
		t.Errorf("expected 0 clients after leave, got %d", r.ClientCount())
	}
}
