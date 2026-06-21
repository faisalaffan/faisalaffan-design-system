package room

import (
	"encoding/json"
	"sync"
)

type Message struct {
	User      string `json:"user"`
	Room      string `json:"room"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type Client struct {
	User string
	Send chan []byte
	Room string
}

type Room struct {
	mu         sync.RWMutex
	Name       string
	clients    map[*Client]bool
	history    []Message
	maxHistory int
	broadcast  chan []byte
	join       chan *Client
	leave      chan *Client
}

func New(name string) *Room {
	r := &Room{
		Name:       name,
		clients:    make(map[*Client]bool),
		history:    make([]Message, 0, 100),
		maxHistory: 100,
		broadcast:  make(chan []byte, 256),
		join:       make(chan *Client),
		leave:      make(chan *Client),
	}
	go r.run()
	return r
}

func (r *Room) run() {
	for {
		select {
		case client := <-r.join:
			r.mu.Lock()
			r.clients[client] = true
			r.mu.Unlock()

		case client := <-r.leave:
			r.mu.Lock()
			delete(r.clients, client)
			close(client.Send)
			r.mu.Unlock()

		case msg := <-r.broadcast:
			var m Message
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			r.mu.Lock()
			r.history = append(r.history, m)
			if len(r.history) > r.maxHistory {
				r.history = r.history[len(r.history)-r.maxHistory:]
			}
			for c := range r.clients {
				select {
				case c.Send <- msg:
				default:
					delete(r.clients, c)
					close(c.Send)
				}
			}
			r.mu.Unlock()
		}
	}
}

func (r *Room) Join(client *Client) {
	r.join <- client
}

func (r *Room) Leave(client *Client) {
	r.leave <- client
}

func (r *Room) Send(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	r.broadcast <- data
	return nil
}

func (r *Room) History() []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h := make([]Message, len(r.history))
	copy(h, r.history)
	return h
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}
