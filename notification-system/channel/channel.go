package channel

import "log"

type Type string

const (
	InApp Type = "in_app"
	Email Type = "email"
	Push  Type = "push"
)

type Notification struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Channel   Type   `json:"channel"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type Sender interface {
	Send(n *Notification) error
	Name() Type
}

type InAppSender struct {
	store *Store
}

func NewInAppSender(store *Store) *InAppSender {
	return &InAppSender{store: store}
}

func (s *InAppSender) Send(n *Notification) error {
	n.Status = "delivered"
	s.store.Save(n)
	return nil
}

func (s *InAppSender) Name() Type { return InApp }

type EmailSender struct{}

func (s *EmailSender) Send(n *Notification) error {
	log.Printf("[EMAIL] To: %s | Subject: %s | Body: %s", n.UserID, n.Title, n.Body)
	n.Status = "sent"
	return nil
}

func (s *EmailSender) Name() Type { return Email }

type PushSender struct{}

func (s *PushSender) Send(n *Notification) error {
	log.Printf("[PUSH] To: %s | Title: %s | Body: %s", n.UserID, n.Title, n.Body)
	n.Status = "sent"
	return nil
}

func (s *PushSender) Name() Type { return Push }

type Store struct {
	notifications []*Notification
}

func NewStore() *Store {
	return &Store{notifications: make([]*Notification, 0)}
}

func (s *Store) Save(n *Notification) {
	s.notifications = append(s.notifications, n)
}

func (s *Store) GetByUser(userID string) []*Notification {
	var result []*Notification
	for _, n := range s.notifications {
		if n.UserID == userID {
			result = append(result, n)
		}
	}
	return result
}

func (s *Store) GetByID(id string) *Notification {
	for _, n := range s.notifications {
		if n.ID == id {
			return n
		}
	}
	return nil
}
