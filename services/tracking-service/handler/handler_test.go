package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func newTestHandler() *Handler {
	rules := model.DefaultValidationRules()
	validator := service.NewLocationValidator(rules)
	fanOut := service.NewFanOut()
	ssHandler := service.NewSSEHandler(fanOut)
	connMgr := service.NewConnectionManager()
	kalmanPool := service.NewKalmanPool()
	ing := &service.Ingestor{
		Validator:  validator,
		FanOut:     fanOut,
		ConnMgr:    connMgr,
		KalmanPool: kalmanPool,
	}
	return New(ing, ssHandler)
}

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	g.GET("/health", HealthCheck)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if body["service"] != "tracking-service" {
		t.Errorf("expected service tracking-service, got %v", body["service"])
	}
}

func TestWebSocketUpgrade_MissingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ws/driver/location", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebSocketUpgrade_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	srv := httptest.NewServer(g)
	defer srv.Close()

	// Convert http URL to ws
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/driver/location?driver_id=drv1&order_id=ord1"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	// Send a valid location update
	update := model.LocationUpdate{
		OrderID:   "ord1",
		DriverID:  "drv1",
		Lat:       -6.200000,
		Lng:       106.816666,
		Speed:     10.0,
		Accuracy:  5.0,
		Bearing:   90.0,
		Timestamp: time.Now(),
	}
	if err := conn.WriteJSON(update); err != nil {
		t.Fatalf("write json: %v", err)
	}

	// Read ack
	var ack map[string]string
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack["status"] != "ok" {
		t.Errorf("expected ok status, got %v", ack["status"])
	}
}

func TestWebSocketUpgrade_InvalidLat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	srv := httptest.NewServer(g)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/driver/location?driver_id=drv2&order_id=ord2"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	// Send invalid lat (out of bounds)
	update := model.LocationUpdate{
		OrderID:   "ord2",
		DriverID:  "drv2",
		Lat:       100.0,
		Lng:       106.816666,
		Speed:     10.0,
		Accuracy:  5.0,
		Bearing:   90.0,
		Timestamp: time.Now(),
	}
	if err := conn.WriteJSON(update); err != nil {
		t.Fatalf("write json: %v", err)
	}

	var ack map[string]string
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack["error"] == "" {
		t.Error("expected error for invalid latitude")
	}
}

func TestSSEStream_MissingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/sse/order/tracking", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebSocket_Throttle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	srv := httptest.NewServer(g)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/driver/location?driver_id=drv3&order_id=ord3"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	now := time.Now()

	// First update — should succeed
	u1 := model.LocationUpdate{
		OrderID:   "ord3",
		DriverID:  "drv3",
		Lat:       -6.200000,
		Lng:       106.816666,
		Speed:     5.0,
		Accuracy:  5.0,
		Timestamp: now,
	}
	if err := conn.WriteJSON(u1); err != nil {
		t.Fatalf("write json: %v", err)
	}
	var ack1 map[string]string
	if err := conn.ReadJSON(&ack1); err != nil {
		t.Fatalf("read ack1: %v", err)
	}
	if ack1["status"] != "ok" {
		t.Errorf("expected ok, got %v", ack1["status"])
	}

	// Second update immediately — should be throttled
	u2 := model.LocationUpdate{
		OrderID:   "ord3",
		DriverID:  "drv3",
		Lat:       -6.210000,
		Lng:       106.826666,
		Speed:     5.0,
		Accuracy:  5.0,
		Timestamp: now.Add(500 * time.Millisecond), // only 500ms later
	}
	if err := conn.WriteJSON(u2); err != nil {
		t.Fatalf("write json: %v", err)
	}
	var ack2 map[string]string
	if err := conn.ReadJSON(&ack2); err != nil {
		t.Fatalf("read ack2: %v", err)
	}
	if ack2["error"] == "" {
		t.Error("expected throttle error")
	}
}

func TestSSEStream_ConnectedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	g := gin.New()
	h.RegisterRoutes(g.Group("/"))

	srv := httptest.NewServer(g)
	defer srv.Close()

	// Subscribe to SSE
	url := srv.URL + "/sse/order/tracking?user_id=usr1&order_id=ord4"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("sse get: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read the initial "connected" event
	buf := make([]byte, 256)
	n, err := resp.Body.Read(buf)
	if err != nil {
		t.Fatalf("read sse: %v", err)
	}
	body := string(buf[:n])
	if !strings.Contains(body, "event: connected") {
		t.Errorf("expected connected event, got: %s", body)
	}
}
