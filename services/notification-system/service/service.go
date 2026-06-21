package service

import (
	"fmt"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/notification-system/channel"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/consistenthash"
)

type Service struct {
	senders map[channel.Type]channel.Sender
	store   *channel.Store
	idRing  *consistenthash.HashRing
	idSeq   int64
}

func New(store *channel.Store) *Service {
	svc := &Service{
		senders: make(map[channel.Type]channel.Sender),
		store:   store,
	}

	inApp := channel.NewInAppSender(store)
	svc.senders[channel.InApp] = inApp
	svc.senders[channel.Email] = &channel.EmailSender{}
	svc.senders[channel.Push] = &channel.PushSender{}

	return svc
}

func (s *Service) Send(userID string, ch channel.Type, title, body string) (*channel.Notification, error) {
	sender, ok := s.senders[ch]
	if !ok {
		return nil, fmt.Errorf("unknown channel: %s", ch)
	}

	s.idSeq++

	n := &channel.Notification{
		ID:        fmt.Sprintf("notif_%d_%d", time.Now().UnixMilli(), s.idSeq),
		UserID:    userID,
		Channel:   ch,
		Title:     title,
		Body:      body,
		CreatedAt: time.Now().UnixMilli(),
	}

	if err := sender.Send(n); err != nil {
		n.Status = "failed"
		return n, err
	}

	return n, nil
}

func (s *Service) GetByUser(userID string) []*channel.Notification {
	return s.store.GetByUser(userID)
}

func (s *Service) GetByID(id string) *channel.Notification {
	return s.store.GetByID(id)
}
