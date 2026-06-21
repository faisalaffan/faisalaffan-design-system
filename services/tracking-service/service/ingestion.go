package service

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins in dev; tighten for production
	},
}

// DriverConn wraps a WebSocket connection for a driver.
type DriverConn struct {
	Conn     *websocket.Conn
	DriverID string
	OrderID  string
	Mu       sync.Mutex
}

// WriteSafe sends a JSON message with connection-level locking.
func (dc *DriverConn) WriteSafe(v interface{}) error {
	dc.Mu.Lock()
	defer dc.Mu.Unlock()
	return dc.Conn.WriteJSON(v)
}

// ConnectionManager tracks all active driver WebSocket connections.
// On reconnect the old connection for the same driver is closed.
type ConnectionManager struct {
	mu       sync.RWMutex
	drivers  map[string]*DriverConn // driverID -> connection
}

// NewConnectionManager creates a connection manager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		drivers: make(map[string]*DriverConn),
	}
}

// Register stores a new driver connection, closing any previous one.
func (cm *ConnectionManager) Register(driverID string, conn *DriverConn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if old, ok := cm.drivers[driverID]; ok {
		log.Printf("ingestion: driver %s reconnecting, closing old connection", driverID)
		old.Conn.Close()
	}
	cm.drivers[driverID] = conn
}

// Unregister removes a driver connection from the map.
func (cm *ConnectionManager) Unregister(driverID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.drivers, driverID)
}

// Get returns the connection for a driver, or nil.
func (cm *ConnectionManager) Get(driverID string) *DriverConn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.drivers[driverID]
}

// Ingestor handles WebSocket upgrades and the read loop for GPS data.
type Ingestor struct {
	Validator  *LocationValidator
	FanOut     *FanOut
	ConnMgr    *ConnectionManager
	KalmanPool *KalmanPool
}

// KalmanPool manages a per-(driver,order) Kalman filter.
type KalmanPool struct {
	mu   sync.RWMutex
	filters map[string]*KalmanFilter // key: "driverID:orderID"
}

// NewKalmanPool creates a pool of Kalman filters.
func NewKalmanPool() *KalmanPool {
	return &KalmanPool{
		filters: make(map[string]*KalmanFilter),
	}
}

func (kp *KalmanPool) key(driverID, orderID string) string {
	return driverID + ":" + orderID
}

// GetOrCreate returns an existing filter or creates a new one.
func (kp *KalmanPool) GetOrCreate(driverID, orderID string) *KalmanFilter {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	key := kp.key(driverID, orderID)
	if f, ok := kp.filters[key]; ok {
		return f
	}
	f := NewKalmanFilter(1.0)
	kp.filters[key] = f
	return f
}

// Remove deletes a filter when the session ends.
func (kp *KalmanPool) Remove(driverID, orderID string) {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	delete(kp.filters, kp.key(driverID, orderID))
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and starts the read loop.
func (ing *Ingestor) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	driverID := r.URL.Query().Get("driver_id")
	orderID := r.URL.Query().Get("order_id")
	if driverID == "" || orderID == "" {
		http.Error(w, "driver_id and order_id are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ingestion: upgrade error: %v", err)
		return
	}

	dc := &DriverConn{
		Conn:     conn,
		DriverID: driverID,
		OrderID:  orderID,
	}
	ing.ConnMgr.Register(driverID, dc)

	// Ping/pong keepalive
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("ingestion: set read deadline: %v", err)
	}

	log.Printf("ingestion: driver %s connected for order %s", driverID, orderID)

	// Read loop
	defer func() {
		conn.Close()
		ing.ConnMgr.Unregister(driverID)
		ing.KalmanPool.Remove(driverID, orderID)
		log.Printf("ingestion: driver %s disconnected from order %s", driverID, orderID)
	}()

	for {
		var u model.LocationUpdate
		if err := conn.ReadJSON(&u); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ingestion: read error driver %s: %v", driverID, err)
			}
			return
		}

		// Fill in query params if not provided in JSON
		if u.DriverID == "" {
			u.DriverID = driverID
		}
		if u.OrderID == "" {
			u.OrderID = orderID
		}

		// Validate
		if err := ing.Validator.Validate(u); err != nil {
			log.Printf("ingestion: validation failed driver %s: %v", driverID, err)
			_ = dc.WriteSafe(map[string]string{"error": err.Error()})
			continue
		}

		// Kalman smoothing
		kf := ing.KalmanPool.GetOrCreate(u.DriverID, u.OrderID)
		smoothed := kf.Update(u.Lat, u.Lng, u.Accuracy, u.Timestamp)
		smoothed.OrderID = u.OrderID
		smoothed.DriverID = u.DriverID
		smoothed.Time = u.Timestamp

		// Publish to fan-out
		ing.FanOut.Publish(u.OrderID, model.SSEEvent{
			Type:    "position",
			Payload: smoothed,
		})

		// Acknowledge to driver
		_ = dc.WriteSafe(map[string]string{
			"status":  "ok",
			"message": "location received",
		})
	}
}

// WaitForConnection is a helper for testing: blocks until a driver connects or timeout.
func (cm *ConnectionManager) WaitForConnection(driverID string, timeout time.Duration) *DriverConn {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return nil
		default:
			if dc := cm.Get(driverID); dc != nil {
				return dc
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
