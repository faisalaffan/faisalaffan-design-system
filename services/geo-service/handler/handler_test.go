package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/service"
	"github.com/gin-gonic/gin"
)

// setupTest creates a fully wired test server and returns the engine + hubStore.
func setupTest(t *testing.T) (*gin.Engine, *HubStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	hubStore := NewHubStore()
	svc := service.New(hubStore)
	h := New(svc, hubStore)

	g := gin.New()
	h.Register(&g.RouterGroup)
	return g, hubStore
}

// parseServiceability decodes a kit.Response wrapper and extracts the
// ServiceabilityResponse from its Data field.
func parseServiceability(t *testing.T, body []byte) *model.ServiceabilityResponse {
	t.Helper()
	var wrap kit.Response
	if err := json.Unmarshal(body, &wrap); err != nil {
		t.Fatalf("parse kit.Response: %v", err)
	}
	data, err := json.Marshal(wrap.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var sr model.ServiceabilityResponse
	if err := json.Unmarshal(data, &sr); err != nil {
		t.Fatalf("parse ServiceabilityResponse: %v", err)
	}
	return &sr
}

// ---------------------------------------------------------------------------
// Hub admin endpoint tests
// ---------------------------------------------------------------------------

func TestRegisterHub_Success(t *testing.T) {
	g, _ := setupTest(t)

	body := `{"id":"hub-1","name":"Central","lat":-6.2088,"lng":106.8456,"max_radius_km":5,"priority":1,"stock":0.9,"rider_avail":0.8,"active":true}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/hubs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterHub_MissingID(t *testing.T) {
	g, _ := setupTest(t)

	body := `{"name":"NoID","lat":-6.2,"lng":106.8,"max_radius_km":5,"active":true}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/hubs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegisterHub_InvalidJSON(t *testing.T) {
	g, _ := setupTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/hubs", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListHubs_Empty(t *testing.T) {
	g, _ := setupTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/hubs", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListHubs_WithData(t *testing.T) {
	g, hubStore := setupTest(t)

	hubStore.Add(&model.Hub{
		ID: "hub-1", Name: "Central", Lat: -6.2088, Lng: 106.8456,
		MaxRadiusKm: 5, Priority: 1, Stock: 0.9, RiderAvail: 0.8, Active: true,
	})
	hubStore.Add(&model.Hub{
		ID: "hub-2", Name: "North", Lat: -6.15, Lng: 106.85,
		MaxRadiusKm: 4, Priority: 2, Stock: 0.7, RiderAvail: 0.6, Active: true,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/hubs", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Serviceability endpoint tests
// ---------------------------------------------------------------------------

func TestServiceability_MissingParams(t *testing.T) {
	g, _ := setupTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/serviceability", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestServiceability_NoHubs(t *testing.T) {
	g, _ := setupTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.8", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	sr := parseServiceability(t, w.Body.Bytes())
	if sr.Serviceable {
		t.Error("expected serviceable=false when no hubs registered")
	}
	if sr.H3Cell == "" {
		t.Error("expected h3_cell even when not serviceable")
	}
}

func TestServiceability_Success(t *testing.T) {
	g, hubStore := setupTest(t)

	hubStore.Add(&model.Hub{
		ID: "hub-1", Name: "Central Jakarta", Lat: -6.2088, Lng: 106.8456,
		MaxRadiusKm: 10, Priority: 1, Stock: 0.9, RiderAvail: 0.8, Active: true,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.84", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sr := parseServiceability(t, w.Body.Bytes())
	if !sr.Serviceable {
		t.Fatal("expected serviceable=true")
	}
	if sr.Hub == nil {
		t.Fatal("expected hub in response")
	}
	if sr.Hub.ID != "hub-1" {
		t.Errorf("expected hub-1, got %s", sr.Hub.ID)
	}
	if sr.H3Cell == "" {
		t.Error("expected h3_cell")
	}
	if sr.DistanceKm <= 0 {
		t.Errorf("expected positive distance, got %f", sr.DistanceKm)
	}
	if sr.ETA == "" {
		t.Error("expected ETA")
	}
	if sr.CacheHit {
		t.Error("expected cache_hit=false on first request")
	}
	if sr.Score <= 0 {
		t.Error("expected positive score")
	}
}

func TestServiceability_OutOfRange(t *testing.T) {
	g, hubStore := setupTest(t)

	hubStore.Add(&model.Hub{
		ID: "hub-1", Name: "Central", Lat: -6.2088, Lng: 106.8456,
		MaxRadiusKm: 1, Priority: 1, Stock: 0.9, RiderAvail: 0.8, Active: true, // 1 km radius
	})

	w := httptest.NewRecorder()
	// ~35 km away from hub
	req, _ := http.NewRequest("GET", "/serviceability?lat=-6.5&lng=107.0", nil)
	g.ServeHTTP(w, req)

	sr := parseServiceability(t, w.Body.Bytes())
	if sr.Serviceable {
		t.Error("expected serviceable=false for out-of-range location")
	}
}

func TestServiceability_InactiveHub(t *testing.T) {
	g, hubStore := setupTest(t)

	hubStore.Add(&model.Hub{
		ID: "hub-1", Name: "Inactive Hub", Lat: -6.2088, Lng: 106.8456,
		MaxRadiusKm: 10, Priority: 1, Stock: 0.9, RiderAvail: 0.8, Active: false,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.84", nil)
	g.ServeHTTP(w, req)

	sr := parseServiceability(t, w.Body.Bytes())
	if sr.Serviceable {
		t.Error("expected serviceable=false when only inactive hubs exist")
	}
}

func TestServiceability_CacheHit(t *testing.T) {
	g, hubStore := setupTest(t)

	hubStore.Add(&model.Hub{
		ID: "hub-1", Name: "Central", Lat: -6.2088, Lng: 106.8456,
		MaxRadiusKm: 10, Priority: 1, Stock: 0.9, RiderAvail: 0.8, Active: true,
	})

	// First request — cache miss
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.84", nil)
	g.ServeHTTP(w1, req1)

	// Second request — expect cache hit
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.84", nil)
	g.ServeHTTP(w2, req2)

	sr := parseServiceability(t, w2.Body.Bytes())
	if !sr.CacheHit {
		t.Error("expected cache_hit=true on second request to same cell")
	}
	if sr.Hub == nil {
		t.Fatal("expected hub on cache hit")
	}
	if sr.Hub.ID != "hub-1" {
		t.Errorf("expected hub-1, got %s", sr.Hub.ID)
	}
}

func TestTieBreaking_HigherStockWins(t *testing.T) {
	g, hubStore := setupTest(t)

	// Hub 1: close but low stock
	hubStore.Add(&model.Hub{
		ID: "hub-close", Name: "Nearby", Lat: -6.21, Lng: 106.84,
		MaxRadiusKm: 10, Priority: 1, Stock: 0.1, RiderAvail: 0.9, Active: true,
	})
	// Hub 2: slightly farther but high stock
	hubStore.Add(&model.Hub{
		ID: "hub-stocked", Name: "Stocked", Lat: -6.19, Lng: 106.85,
		MaxRadiusKm: 10, Priority: 1, Stock: 0.95, RiderAvail: 0.5, Active: true,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/serviceability?lat=-6.2&lng=106.84", nil)
	g.ServeHTTP(w, req)

	sr := parseServiceability(t, w.Body.Bytes())
	if !sr.Serviceable {
		t.Fatal("expected serviceable=true")
	}
	if sr.Hub == nil {
		t.Fatal("expected hub in response")
	}
	if sr.Score <= 0 {
		t.Errorf("expected positive score, got %f", sr.Score)
	}
}
