package service

import (
	"log"
	"sync"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
)

// Subscriber receives SSE events for a specific order.
type Subscriber struct {
	ID      string
	Events  chan model.SSEEvent
	CloseCh chan struct{}
}

// OrderUpdatePipe serialises all updates for a single order through a single
// consumer goroutine, guaranteeing in-order delivery to all subscribers.
type OrderUpdatePipe struct {
	OrderID     string
	events      chan model.SSEEvent
	subscribers map[string]*Subscriber
	mu          sync.RWMutex
	done        chan struct{}
	once        sync.Once
}

// NewOrderUpdatePipe creates a pipe that spawns a single consumer goroutine.
func NewOrderUpdatePipe(orderID string) *OrderUpdatePipe {
	p := &OrderUpdatePipe{
		OrderID:     orderID,
		events:      make(chan model.SSEEvent, 64),
		subscribers: make(map[string]*Subscriber),
		done:        make(chan struct{}),
	}
	go p.consume()
	return p
}

// consume is the single goroutine that fans events out to all subscribers.
func (p *OrderUpdatePipe) consume() {
	for e := range p.events {
		p.mu.RLock()
		for id, sub := range p.subscribers {
			select {
			case sub.Events <- e:
			default:
				// Non-blocking push: drop if subscriber buffer is full.
				// Newer positions are more valuable than older ones.
				log.Printf("fanout: dropping event for subscriber %s (order %s) — buffer full", id, p.OrderID)
			}
		}
		p.mu.RUnlock()
	}
	// Channel closed — notify all subscribers
	p.mu.Lock()
	for id, sub := range p.subscribers {
		close(sub.CloseCh)
		delete(p.subscribers, id)
	}
	p.mu.Unlock()
}

// Subscribe adds a subscriber to this order's pipe.
func (p *OrderUpdatePipe) Subscribe(sub *Subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subscribers[sub.ID] = sub
}

// Unsubscribe removes a subscriber.
func (p *OrderUpdatePipe) Unsubscribe(subID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if sub, ok := p.subscribers[subID]; ok {
		close(sub.CloseCh)
		delete(p.subscribers, subID)
	}
}

// Publish enqueues an event for fan-out.
func (p *OrderUpdatePipe) Publish(evt model.SSEEvent) {
	select {
	case p.events <- evt:
	default:
		log.Printf("fanout: pipe buffer full for order %s, dropping event %s", p.OrderID, evt.Type)
	}
}

// SubscriberCount returns the number of active subscribers.
func (p *OrderUpdatePipe) SubscriberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subscribers)
}

// Close shuts down the pipe.
func (p *OrderUpdatePipe) Close() {
	p.once.Do(func() {
		close(p.events)
		<-p.done // wait for consumer to finish
	})
}

// FanOut manages per-order pipes.
type FanOut struct {
	mu    sync.RWMutex
	pipes map[string]*OrderUpdatePipe // orderID -> pipe
}

// NewFanOut creates a fan-out manager.
func NewFanOut() *FanOut {
	return &FanOut{
		pipes: make(map[string]*OrderUpdatePipe),
	}
}

// GetOrCreatePipe returns the pipe for an order, creating one if needed.
func (fo *FanOut) GetOrCreatePipe(orderID string) *OrderUpdatePipe {
	fo.mu.RLock()
	p, ok := fo.pipes[orderID]
	fo.mu.RUnlock()
	if ok {
		return p
	}

	fo.mu.Lock()
	defer fo.mu.Unlock()
	// Double-check after acquiring write lock
	if p, ok := fo.pipes[orderID]; ok {
		return p
	}
	p = NewOrderUpdatePipe(orderID)
	fo.pipes[orderID] = p
	return p
}

// Publish sends an event to the order's pipe.
func (fo *FanOut) Publish(orderID string, evt model.SSEEvent) {
	p := fo.GetOrCreatePipe(orderID)
	p.Publish(evt)
}

// Subscribe adds a subscriber to an order.
func (fo *FanOut) Subscribe(orderID string, sub *Subscriber) {
	p := fo.GetOrCreatePipe(orderID)
	p.Subscribe(sub)
}

// Unsubscribe removes a subscriber from an order.
func (fo *FanOut) Unsubscribe(orderID, subID string) {
	fo.mu.RLock()
	p, ok := fo.pipes[orderID]
	fo.mu.RUnlock()
	if ok {
		p.Unsubscribe(subID)
	}
}
