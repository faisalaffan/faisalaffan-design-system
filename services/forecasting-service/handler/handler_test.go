package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
	"github.com/gin-gonic/gin"
)

// mockService implements ForecastingOrchestrator for testing.
type mockService struct {
	forecastResult model.ForecastResult
	forecastOK     bool
	replenResult   model.ReplenishmentResult
	replenOK       bool
	stats          model.SKUForecastStats
	degraded       []string
}

func (m *mockService) RunDailyForecast(_ context.Context, hubIDs []string) ([]model.ForecastResponse, error) {
	resp := model.ForecastResponse{
		SKU:   "FNB-001",
		HubID: hubIDs[0],
	}
	return []model.ForecastResponse{resp}, nil
}

func (m *mockService) GetForecast(_, _ string) (model.ForecastResult, bool) {
	return m.forecastResult, m.forecastOK
}

func (m *mockService) GetReplenishment(_, _ string) (model.ReplenishmentResult, bool) {
	return m.replenResult, m.replenOK
}

func (m *mockService) GetAccuracyStats(_, _ string) model.SKUForecastStats {
	return m.stats
}

func (m *mockService) GetDegradedSKUs() []string {
	return m.degraded
}

func (m *mockService) SeedExternalFeatures(_, _ string, _ model.ExternalFeature) {}

func setupRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func TestRunForecast(t *testing.T) {
	h := New(&mockService{})
	r := setupRouter(h)

	body := `{"hub_ids":["JKT-01","BDO-01"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forecast/run", strings.NewReader(body))
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
}

func TestRunForecast_EmptyBody(t *testing.T) {
	h := New(&mockService{})
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/forecast/run", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetForecast_Found(t *testing.T) {
	fc := model.ForecastResult{
		SKU:   "FNB-001",
		HubID: "JKT-01",
		Method: "holtwinters_multiplicative",
	}
	h := New(&mockService{forecastResult: fc, forecastOK: true})
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/forecast/FNB-001/JKT-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
}

func TestGetForecast_NotFound(t *testing.T) {
	h := New(&mockService{forecastOK: false})
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/forecast/UNKNOWN/XYZ", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetForecastAccuracy(t *testing.T) {
	stats := model.SKUForecastStats{
		DataPoints: 15,
		WeeklyMAPE: 12.5,
	}
	h := New(&mockService{stats: stats})
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/forecast/accuracy/FNB-001/JKT-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
}

func TestSeedFeatures(t *testing.T) {
	h := New(&mockService{})
	r := setupRouter(h)

	body := `{"sku":"FNB-001","hub_id":"JKT-01","temperature":32.5,"humidity":70,"is_raining":false}`
	req := httptest.NewRequest(http.MethodPost, "/admin/features", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDegraded(t *testing.T) {
	h := New(&mockService{degraded: []string{"FNB-001|JKT-01"}})
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/admin/degraded", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// kit.Response mirrors kit.Response for test deserialisation.
var _ = func() int {
	// Compile-time check that our service types are accessible.
	_ = model.DefaultHoltWintersParams()
	return 0
}()
