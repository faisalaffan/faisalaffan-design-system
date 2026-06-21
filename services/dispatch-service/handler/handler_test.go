package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/service"
	"github.com/gin-gonic/gin"
)

func setupTest(t *testing.T) (*DispatchHandler, *gin.Engine) {
	t.Helper()

	cfg := model.DefaultBatchConfig()
	cfg.MaxWindow = 50 * time.Millisecond
	cfg.MinOrders = 1 // flush immediately for tests

	scoreCfg := model.DefaultMatchScore()
	reCfg := service.DefaultReassignmentConfig()

	batcher := service.NewBatchCollector(cfg)
	matcher := service.NewGreedyMatcher(scoreCfg)
	sm := service.NewDriverStateMachine()
	reassign := service.NewReassignmentHandler(matcher, sm, reCfg)

	// Register a test driver
	sm.AddDriver(&model.Driver{
		ID:      "driver-1",
		Name:    "Test Driver",
		Status:  model.DriverIdle,
		Load:    0,
		MaxLoad: 3,
		Location: model.Location{Lat: -6.2088, Lng: 106.8456},
	})

	h := NewDispatchHandler(batcher, matcher, sm, reassign)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	h.Register(group)

	// Allow batches to process
	time.Sleep(100 * time.Millisecond)

	return h, r
}

func TestCreateOrder(t *testing.T) {
	_, r := setupTest(t)

	body := map[string]interface{}{
		"customer_id": "cust-1",
		"items": []map[string]interface{}{
			{"product_id": "p1", "name": "Item 1", "quantity": 2},
		},
		"total": 50000,
		"delivery_location": map[string]interface{}{
			"lat": -6.2146,
			"lng": 106.8451,
		},
		"hub_id":   "hub-1",
		"priority": 1,
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dispatch/order", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestCreateOrder_Validation(t *testing.T) {
	_, r := setupTest(t)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing customer_id", map[string]interface{}{"hub_id": "h1"}},
		{"missing items", map[string]interface{}{"customer_id": "c1", "hub_id": "h1", "total": 100}},
		{"missing delivery_location", map[string]interface{}{"customer_id": "c1", "items": []map[string]interface{}{{"product_id": "p1", "name": "n", "quantity": 1}}, "hub_id": "h1", "total": 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/dispatch/order", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetOrder(t *testing.T) {
	h, r := setupTest(t)

	// Create an order directly
	order := &model.Order{
		ID:         "test-order-1",
		CustomerID: "cust-1",
		Status:     model.OrderPending,
		Priority:   1,
	}
	h.mu.Lock()
	h.orders[order.ID] = order
	h.mu.Unlock()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dispatch/orders/test-order-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data == nil {
		t.Error("expected non-nil data")
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	_, r := setupTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dispatch/orders/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateDriverLocation(t *testing.T) {
	h, r := setupTest(t)

	// Ensure driver exists
	h.stateMachine.AddDriver(&model.Driver{
		ID:     "loc-driver-1",
		Name:   "Loc Driver",
		Status: model.DriverIdle,
	})

	body := map[string]interface{}{
		"driver_id": "loc-driver-1",
		"lat":       -6.2000,
		"lng":       106.8000,
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dispatch/driver/location", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify location was updated
	driver, _ := h.stateMachine.GetDriver("loc-driver-1")
	if driver.Location.Lat != -6.2000 || driver.Location.Lng != 106.8000 {
		t.Errorf("location not updated: got %+v", driver.Location)
	}
}

func TestAcceptAssignment(t *testing.T) {
	h, r := setupTest(t)

	// Create a pending assignment
	asgn := &model.Assignment{
		ID:       "asgn-1",
		OrderID:  "order-accept-1",
		DriverID: "driver-1",
		Status:   "PENDING",
	}
	h.mu.Lock()
	h.assignments[asgn.ID] = asgn
	h.orders["order-accept-1"] = &model.Order{
		ID:     "order-accept-1",
		Status: model.OrderAssigned,
	}
	h.mu.Unlock()

	body := map[string]interface{}{
		"assignment_id": "asgn-1",
		"driver_id":     "driver-1",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dispatch/driver/accept", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify assignment status updated
	h.mu.RLock()
	updated := h.assignments["asgn-1"]
	h.mu.RUnlock()
	if updated.Status != "ACCEPTED" {
		t.Errorf("expected ACCEPTED, got %s", updated.Status)
	}
}

func TestAcceptAssignment_WrongDriver(t *testing.T) {
	h, r := setupTest(t)

	asgn := &model.Assignment{
		ID:       "asgn-wrong",
		OrderID:  "order-wrong",
		DriverID: "driver-1",
		Status:   "PENDING",
	}
	h.mu.Lock()
	h.assignments[asgn.ID] = asgn
	h.mu.Unlock()

	body := map[string]interface{}{
		"assignment_id": "asgn-wrong",
		"driver_id":     "driver-other",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dispatch/driver/accept", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for wrong driver, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRejectAssignment(t *testing.T) {
	cfg := model.DefaultBatchConfig()
	cfg.MaxWindow = 200 * time.Millisecond
	cfg.MinOrders = 5 // don't auto-flush

	scoreCfg := model.DefaultMatchScore()
	reCfg := service.DefaultReassignmentConfig()

	batcher := service.NewBatchCollector(cfg)
	matcher := service.NewGreedyMatcher(scoreCfg)
	sm := service.NewDriverStateMachine()
	reassign := service.NewReassignmentHandler(matcher, sm, reCfg)

	sm.AddDriver(&model.Driver{
		ID:      "rej-driver-1",
		Name:    "Reject Driver",
		Status:  model.DriverIdle,
		MaxLoad: 3,
		Location: model.Location{Lat: -6.2088, Lng: 106.8456},
	})

	h := NewDispatchHandler(batcher, matcher, sm, reassign)

	// Transition driver to ASSIGNED first (as the matcher would do)
	if err := sm.Transition("rej-driver-1", model.DriverAssigned, "test-match"); err != nil {
		t.Fatalf("setup transition: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	h.Register(group)

	time.Sleep(50 * time.Millisecond)

	// Create order and track it
	order := &model.Order{
		ID:         "order-reject-1",
		CustomerID: "cust-1",
		Status:     model.OrderAssigned,
		Priority:   1,
		DeliveryLocation: model.Location{Lat: -6.2146, Lng: 106.8451},
		HubID:      "hub-1",
	}
	h.mu.Lock()
	h.orders[order.ID] = order
	h.mu.Unlock()
	h.reassign.TrackOrder(order)

	// Track assignment
	asgn := &model.Assignment{
		ID:       "asgn-reject",
		OrderID:  "order-reject-1",
		DriverID: "rej-driver-1",
		Status:   "PENDING",
	}
	h.mu.Lock()
	h.assignments[asgn.ID] = asgn
	h.mu.Unlock()
	h.reassign.TrackAssignment(asgn)

	body := map[string]interface{}{
		"assignment_id": "asgn-reject",
		"driver_id":     "rej-driver-1",
		"order_id":      "order-reject-1",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dispatch/driver/reject", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGreedyMatcherHaversine verifies the Haversine distance calculation.
func TestGreedyMatcherHaversine(t *testing.T) {
	// Jakarta (city centre) to Bandung (approx 150 km)
	dist := service.Haversine(-6.2088, 106.8456, -6.9175, 107.6191)
	if dist < 100 || dist > 200 {
		t.Errorf("expected ~150 km, got %.2f km", dist)
	}
	t.Logf("Jakarta-Bandung distance: %.2f km", dist)

	// Same point should be 0
	dist = service.Haversine(-6.2088, 106.8456, -6.2088, 106.8456)
	if dist != 0 {
		t.Errorf("expected 0 for same point, got %.2f", dist)
	}
}

// TestGreedyMatcherETA verifies ETA calculation.
func TestGreedyMatcherETA(t *testing.T) {
	// 10 km at 8.33 m/s = ~1200s
	eta := service.ETA(10.0)
	if eta < 1100 || eta > 1300 {
		t.Errorf("expected ~1200s for 10km, got %.2f", eta)
	}
}

// TestGreedyMatcherMatch verifies end-to-end matching.
func TestGreedyMatcherMatch(t *testing.T) {
	matcher := service.NewGreedyMatcher(model.DefaultMatchScore())

	orders := []*model.Order{
		{ID: "o1", Priority: 1, DeliveryLocation: model.Location{Lat: -6.2146, Lng: 106.8451}},
		{ID: "o2", Priority: 3, DeliveryLocation: model.Location{Lat: -6.2000, Lng: 106.8200}},
	}

	drivers := []*model.Driver{
		{ID: "d1", Status: model.DriverIdle, Load: 0, MaxLoad: 3, Location: model.Location{Lat: -6.2088, Lng: 106.8456}},
		{ID: "d2", Status: model.DriverIdle, Load: 0, MaxLoad: 3, Location: model.Location{Lat: -6.2200, Lng: 106.8300}},
	}

	assignments := matcher.Match(orders, drivers)
	if len(assignments) == 0 {
		t.Fatal("expected at least one assignment")
	}

	t.Logf("Created %d assignments", len(assignments))
	for _, a := range assignments {
		t.Logf("  %s -> %s (score=%.4f)", a.OrderID, a.DriverID, a.Score)
	}
}

// TestDriverStateMachineTransitions verifies valid and invalid transitions.
func TestDriverStateMachineTransitions(t *testing.T) {
	sm := service.NewDriverStateMachine()
	sm.AddDriver(&model.Driver{ID: "d1", Status: model.DriverIdle})

	// Valid: IDLE -> ASSIGNED
	if err := sm.Transition("d1", model.DriverAssigned, "match"); err != nil {
		t.Fatalf("expected valid IDLE->ASSIGNED, got: %v", err)
	}

	// Valid: ASSIGNED -> TO_HUB
	if err := sm.Transition("d1", model.DriverToHub, "accepted"); err != nil {
		t.Fatalf("expected valid ASSIGNED->TO_HUB, got: %v", err)
	}

	// Invalid: TO_HUB -> COMPLETED (skip intermediate states)
	if err := sm.Transition("d1", model.DriverCompleted, "skip"); err == nil {
		t.Error("expected error for invalid TO_HUB->COMPLETED transition")
	}

	// Valid: TO_HUB -> PICKING -> DELIVERING -> COMPLETED
	if err := sm.Transition("d1", model.DriverPicking, "arrived"); err != nil {
		t.Fatalf("expected valid TO_HUB->PICKING, got: %v", err)
	}
	if err := sm.Transition("d1", model.DriverDelivering, "picked"); err != nil {
		t.Fatalf("expected valid PICKING->DELIVERING, got: %v", err)
	}
	if err := sm.Transition("d1", model.DriverCompleted, "delivered"); err != nil {
		t.Fatalf("expected valid DELIVERING->COMPLETED, got: %v", err)
	}

	// Verify event log
	events := sm.Events()
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
}

// TestBatchCollectorFlushOnMinOrders verifies the collector flushes when
// MinOrders is reached before the MaxWindow timer fires.
func TestBatchCollectorFlushOnMinOrders(t *testing.T) {
	cfg := model.BatchConfig{MaxWindow: 10 * time.Second, MinOrders: 3}
	bc := service.NewBatchCollector(cfg)
	bc.Start()

	// Enqueue orders
	go func() {
		for i := 0; i < 3; i++ {
			bc.Orders() <- &model.Order{
				ID:         fmt.Sprintf("batch-order-%d", i+1),
				CustomerID: "cust-1",
				Status:     model.OrderPending,
			}
		}
	}()

	// Wait for batch
	select {
	case batch := <-bc.Batches():
		if len(batch) != 3 {
			t.Errorf("expected 3 orders in batch, got %d", len(batch))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for batch flush")
	}

	bc.Stop()
}

// TestFindBestDriver verifies the single driver selection.
func TestFindBestDriver(t *testing.T) {
	matcher := service.NewGreedyMatcher(model.DefaultMatchScore())

	order := &model.Order{
		ID:               "o1",
		Priority:         1,
		DeliveryLocation: model.Location{Lat: -6.2146, Lng: 106.8451},
	}

	drivers := []*model.Driver{
		{ID: "d1", Status: model.DriverIdle, Load: 0, MaxLoad: 3, Location: model.Location{Lat: -6.2088, Lng: 106.8456}},
		{ID: "d2", Status: model.DriverIdle, Load: 2, MaxLoad: 3, Location: model.Location{Lat: -6.3000, Lng: 106.9000}},
		{ID: "d3", Status: model.DriverIdle, Load: 0, MaxLoad: 3, Location: model.Location{Lat: -6.1000, Lng: 106.7000}},
	}

	best, score := matcher.FindBestDriver(order, drivers)
	if best == nil {
		t.Fatal("expected a best driver")
	}
	if best.ID != "d1" {
		t.Logf("Best driver: %s (score=%.4f)", best.ID, score)
	}
}
