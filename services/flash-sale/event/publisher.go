package event

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

type OrderCreatedEvent struct {
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	ProductID     string `json:"product_id"`
	Quantity      int    `json:"quantity"`
	ReservationID string `json:"reservation_id"`
	DeviceFP      string `json:"device_fp"`
	Timestamp     int64  `json:"timestamp"`
	EventType     string `json:"event_type"`
}

type Publisher interface {
	PublishOrderCreated(ctx context.Context, event OrderCreatedEvent) error
}

// ChannelPublisher publishes events to a Go channel (stand-in for Kafka).
type ChannelPublisher struct {
	ch chan<- []byte
}

func NewChannelPublisher(ch chan<- []byte) *ChannelPublisher {
	return &ChannelPublisher{ch: ch}
}

func (p *ChannelPublisher) PublishOrderCreated(ctx context.Context, event OrderCreatedEvent) error {
	event.EventType = "order.created"
	event.Timestamp = time.Now().UnixMilli()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	select {
	case p.ch <- data:
		log.Printf("event: order.created %s", event.OrderID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		log.Printf("warn: event channel full, dropping order.created %s", event.OrderID)
		return nil // non-blocking — don't fail checkout for event delivery
	}
}

// LogPublisher logs events (fallback when no message broker).
type LogPublisher struct{}

func (p *LogPublisher) PublishOrderCreated(ctx context.Context, event OrderCreatedEvent) error {
	event.EventType = "order.created"
	event.Timestamp = time.Now().UnixMilli()
	data, _ := json.Marshal(event)
	log.Printf("event: %s", string(data))
	return nil
}
