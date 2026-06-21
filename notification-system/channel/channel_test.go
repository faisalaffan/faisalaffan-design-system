package channel

import (
	"testing"
)

func TestInAppSender_Send(t *testing.T) {
	store := NewStore()
	s := NewInAppSender(store)
	n := &Notification{ID: "1", UserID: "u1", Channel: InApp, Title: "T", Body: "B"}
	s.Send(n)
	if n.Status != "delivered" {
		t.Errorf("expected delivered, got %s", n.Status)
	}
	if len(store.notifications) != 1 {
		t.Errorf("expected 1 notification in store, got %d", len(store.notifications))
	}
}

func TestStore_GetByUser(t *testing.T) {
	store := NewStore()
	store.Save(&Notification{ID: "1", UserID: "a"})
	store.Save(&Notification{ID: "2", UserID: "b"})
	store.Save(&Notification{ID: "3", UserID: "a"})

	results := store.GetByUser("a")
	if len(results) != 2 {
		t.Errorf("expected 2 notifications for user a, got %d", len(results))
	}
}

func TestStore_GetByID(t *testing.T) {
	store := NewStore()
	store.Save(&Notification{ID: "abc", UserID: "x"})

	n := store.GetByID("abc")
	if n == nil {
		t.Fatal("expected notification, got nil")
	}
	if n.UserID != "x" {
		t.Errorf("expected user x, got %s", n.UserID)
	}
}

func TestEmailSender_Send(t *testing.T) {
	s := &EmailSender{}
	n := &Notification{ID: "e1", UserID: "u1", Channel: Email, Title: "T", Body: "B"}
	s.Send(n)
	if n.Status != "sent" {
		t.Errorf("expected sent, got %s", n.Status)
	}
}
