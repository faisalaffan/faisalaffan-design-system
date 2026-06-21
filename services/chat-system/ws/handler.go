package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/chat-system/room"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Handler struct {
	manager *room.Manager
}

func NewHandler(mgr *room.Manager) *Handler {
	return &Handler{manager: mgr}
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	roomName := r.URL.Query().Get("room")
	user := r.URL.Query().Get("user")
	if roomName == "" || user == "" {
		http.Error(w, "room and user query params required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	rm := h.manager.GetOrCreate(roomName)
	client := &room.Client{
		User: user,
		Send: make(chan []byte, 256),
		Room: roomName,
	}
	rm.Join(client)

	// Send history
	for _, msg := range rm.History() {
		data, _ := json.Marshal(msg)
		client.Send <- data
	}

	go h.writePump(conn, client)
	h.readPump(conn, client, rm)
}

func (h *Handler) readPump(conn *websocket.Conn, client *room.Client, rm *room.Room) {
	defer func() {
		rm.Leave(client)
		conn.Close()
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		msg := room.Message{
			User:      client.User,
			Room:      client.Room,
			Content:   string(msgBytes),
			Timestamp: time.Now().UnixMilli(),
		}
		rm.Send(msg)
	}
}

func (h *Handler) writePump(conn *websocket.Conn, client *room.Client) {
	defer conn.Close()
	for msg := range client.Send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}
