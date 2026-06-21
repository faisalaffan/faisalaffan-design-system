package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/service"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := model.DefaultConfig()
	svc := service.NewETAService(cfg, nil)
	h := NewETAHandler(svc)
	h.Register(&r.RouterGroup)
	return r
}

func TestCalculateETA_Success(t *testing.T) {
	r := setupRouter()

	body := `{
		"order_id": "test-1",
		"hub_id": "hub-1",
		"item_count": 5,
		"hub_lat": -6.200000,
		"hub_lng": 106.816666,
		"cust_lat": -6.250000,
		"cust_lng": 106.850000,
		"queue_depth": 3,
		"drivers_avail": 5,
		"hour": 14,
		"weekday": 3
	}`

	req, _ := http.NewRequest("POST", "/eta/calculate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("response data is not an object")
	}
	if data["order_id"] != "test-1" {
		t.Errorf("expected order_id test-1, got %v", data["order_id"])
	}
	if _, exists := data["eta_p50_s"]; !exists {
		t.Error("missing eta_p50_s field")
	}
	if _, exists := data["components"]; !exists {
		t.Error("missing components field")
	}
}

func TestCalculateETA_InvalidRequest(t *testing.T) {
	r := setupRouter()

	body := `{"order_id": "test-2"}`

	req, _ := http.NewRequest("POST", "/eta/calculate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetETA_NotFound(t *testing.T) {
	r := setupRouter()

	req, _ := http.NewRequest("GET", "/eta/non-existent-order", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
