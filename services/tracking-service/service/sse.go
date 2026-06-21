package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
	"github.com/google/uuid"
)

// SSEHandler manages Server-Sent Events streams for order tracking.
type SSEHandler struct {
	FanOut *FanOut
}

// NewSSEHandler creates an SSE handler.
func NewSSEHandler(fo *FanOut) *SSEHandler {
	return &SSEHandler{FanOut: fo}
}

// ServeHTTP handles GET /sse/order/tracking?user_id=X&order_id=Y.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	orderID := r.URL.Query().Get("order_id")
	if userID == "" || orderID == "" {
		http.Error(w, "user_id and order_id are required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	subID := uuid.New().String()
	sub := &Subscriber{
		ID:      subID,
		Events:  make(chan model.SSEEvent, 32),
		CloseCh: make(chan struct{}),
	}
	h.FanOut.Subscribe(orderID, sub)

	log.Printf("sse: user %s subscribed to order %s (sub %s)", userID, orderID, subID)

	defer func() {
		h.FanOut.Unsubscribe(orderID, subID)
		log.Printf("sse: user %s unsubscribed from order %s (sub %s)", userID, orderID, subID)
	}()

	// Send initial "connected" event
	writeSSE(w, "connected", fmt.Sprintf(`{"user_id":"%s","order_id":"%s"}`, userID, orderID))
	flusher.Flush()

	// Keepalive ticker
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	// Use request context for cancellation
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.CloseCh:
			return
		case evt := <-sub.Events:
			data, err := json.Marshal(evt.Payload)
			if err != nil {
				log.Printf("sse: marshal error: %v", err)
				continue
			}
			writeSSE(w, evt.Type, string(data))
			flusher.Flush()
		case <-keepalive.C:
			// Keepalive comment to prevent proxies from closing the connection
			_, err := io.WriteString(w, ": keepalive\n\n")
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE writes a formatted SSE event to the response writer.
func writeSSE(w io.Writer, eventType, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
}
