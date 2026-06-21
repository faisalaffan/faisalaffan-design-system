package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func setupTestHandler(t *testing.T) (*InventoryHandler, *gin.Engine) {
	t.Helper()

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	// Flush DB before each test
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	repo := repository.NewInventoryRepo(rdb)
	svc := service.NewInventoryService(repo)
	h := NewInventoryHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	h.Register(group)

	return h, r
}

func TestGetStock(t *testing.T) {
	_, r := setupTestHandler(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stock/hub1/sku123", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	// resp.Data is decoded as map[string]interface{} via json.Unmarshal
	_ = data
	if !ok {
		// The response might have data directly as an object — check it's there
		if resp.Data == nil {
			t.Error("expected non-nil data")
		}
	}
}

func TestSetStock(t *testing.T) {
	_, r := setupTestHandler(t)

	body := map[string]interface{}{
		"hub_id":   "hub1",
		"sku":      "sku123",
		"quantity": 100,
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetStock_Validation(t *testing.T) {
	_, r := setupTestHandler(t)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing hub_id", map[string]interface{}{"sku": "s1", "quantity": 10}},
		{"missing sku", map[string]interface{}{"hub_id": "h1", "quantity": 10}},
		{"negative quantity", map[string]interface{}{"hub_id": "h1", "sku": "s1", "quantity": -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestReserve(t *testing.T) {
	_, r := setupTestHandler(t)

	// First set some stock
	setBody := map[string]interface{}{
		"hub_id":   "hub1",
		"sku":      "sku123",
		"quantity": 50,
	}
	b, _ := json.Marshal(setBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set stock: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Reserve
	reserveBody := model.ReserveRequest{
		HubID:  "hub1",
		CartID: "cart1",
		UserID: "user1",
		Items: []model.ReserveItem{
			{SKU: "sku123", Quantity: 10},
		},
	}
	b, _ = json.Marshal(reserveBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/reserve", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestReserve_InsufficientStock(t *testing.T) {
	_, r := setupTestHandler(t)

	setBody := map[string]interface{}{
		"hub_id":   "hub1",
		"sku":      "sku456",
		"quantity": 5,
	}
	b, _ := json.Marshal(setBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set stock: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Try to reserve more than available
	reserveBody := model.ReserveRequest{
		HubID:  "hub1",
		CartID: "cart1",
		UserID: "user1",
		Items: []model.ReserveItem{
			{SKU: "sku456", Quantity: 10},
		},
	}
	b, _ = json.Marshal(reserveBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/reserve", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRelease(t *testing.T) {
	_, r := setupTestHandler(t)

	// Set stock
	setBody := map[string]interface{}{
		"hub_id":   "hub1",
		"sku":      "sku789",
		"quantity": 30,
	}
	b, _ := json.Marshal(setBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Reserve
	reserveBody := model.ReserveRequest{
		HubID:  "hub1",
		CartID: "cart1",
		UserID: "user1",
		Items:  []model.ReserveItem{{SKU: "sku789", Quantity: 5}},
	}
	b, _ = json.Marshal(reserveBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/reserve", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var reserveResp kit.Response
	json.Unmarshal(w.Body.Bytes(), &reserveResp)
	data := reserveResp.Data.(map[string]interface{})
	reservationID := data["reservation_id"].(string)

	// Release
	releaseBody := model.ReleaseRequest{ReservationID: reservationID}
	b, _ = json.Marshal(releaseBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/release?hub_id=hub1&sku=sku789", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirm(t *testing.T) {
	_, r := setupTestHandler(t)

	// Set stock
	setBody := map[string]interface{}{
		"hub_id":   "hub1",
		"sku":      "sku111",
		"quantity": 20,
	}
	b, _ := json.Marshal(setBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/stock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Reserve
	reserveBody := model.ReserveRequest{
		HubID:  "hub1",
		CartID: "cart1",
		UserID: "user1",
		Items:  []model.ReserveItem{{SKU: "sku111", Quantity: 3}},
	}
	b, _ = json.Marshal(reserveBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/reserve", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var reserveResp kit.Response
	json.Unmarshal(w.Body.Bytes(), &reserveResp)
	data := reserveResp.Data.(map[string]interface{})
	reservationID := data["reservation_id"].(string)

	// Confirm
	confirmBody := model.ConfirmRequest{ReservationID: reservationID}
	b, _ = json.Marshal(confirmBody)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/confirm?hub_id=hub1&sku=sku111", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
