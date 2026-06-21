package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/gin-gonic/gin"
)

// ensure mockService implements FlashSaleService at compile time
var _ FlashSaleService = (*mockService)(nil)

// mockService implements FlashSaleService for unit tests.
type mockService struct {
	checkoutFn    func(context.Context, model.CheckoutRequest) (*model.CheckoutResponse, error)
	queueStatusFn func(context.Context, string, string) (*model.QueueStatusResponse, error)
	dryRunFn      func(context.Context, model.CheckoutRequest) (*model.DryRunResponse, error)
}

func (m *mockService) Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error) {
	return m.checkoutFn(ctx, req)
}

func (m *mockService) QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error) {
	return m.queueStatusFn(ctx, productID, userID)
}
func (m *mockService) ReleaseReservation(ctx context.Context, reservationID string) error { return nil }
func (m *mockService) ConfirmReservation(ctx context.Context, reservationID string) error { return nil }
func (m *mockService) DryRun(ctx context.Context, req model.CheckoutRequest) (*model.DryRunResponse, error) {
	if m.dryRunFn != nil {
		return m.dryRunFn(ctx, req)
	}
	return &model.DryRunResponse{WouldSucceed: true}, nil
}

func setupTest(mock *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	h := New(mock, nil, "", nil) // repo=nil, cb=nil for mock tests
	g.POST("/flash-sale/checkout", h.Checkout)
	g.POST("/flash-sale/release", h.Release)
	g.POST("/flash-sale/confirm", h.Confirm)
	g.GET("/flash-sale/queue-status", h.QueueStatus)
	g.POST("/flash-sale/dry-run", h.DryRun)
	g.GET("/flash-sale/metrics", h.Metrics)
	g.GET("/flash-sale/health", h.Health)
	return g
}

func postJSON(t *testing.T, g *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(w, req)
	return w
}

func TestCheckout_Success(t *testing.T) {
	mock := &mockService{
		checkoutFn: func(_ context.Context, _ model.CheckoutRequest) (*model.CheckoutResponse, error) {
			return &model.CheckoutResponse{OrderID: "ORD-test-1", Status: model.StatusCompleted}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/checkout", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1, IdempotencyKey: "idem-test-1",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(resp.Data)
	var cr model.CheckoutResponse
	json.Unmarshal(data, &cr)
	if cr.OrderID != "ORD-test-1" || cr.Status != model.StatusCompleted {
		t.Fatalf("unexpected response: %+v", cr)
	}
}

func TestCheckout_InvalidAttestation(t *testing.T) {
	mock := &mockService{
		checkoutFn: func(_ context.Context, _ model.CheckoutRequest) (*model.CheckoutResponse, error) {
			return &model.CheckoutResponse{Status: model.StatusInvalidAttestation}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/checkout", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "bad", ExpiresAt: 2000000000, Quantity: 1,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCheckout_RateLimited(t *testing.T) {
	mock := &mockService{
		checkoutFn: func(_ context.Context, _ model.CheckoutRequest) (*model.CheckoutResponse, error) {
			return &model.CheckoutResponse{Status: model.StatusRateLimited}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/checkout", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1, IdempotencyKey: "idem-test-1",
	})

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestCheckout_Queued(t *testing.T) {
	mock := &mockService{
		checkoutFn: func(_ context.Context, _ model.CheckoutRequest) (*model.CheckoutResponse, error) {
			return &model.CheckoutResponse{Position: 5, Status: model.StatusQueued}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/checkout", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1, IdempotencyKey: "idem-test-1",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr model.CheckoutResponse
	json.Unmarshal(data, &cr)
	if cr.Position != 5 || cr.Status != model.StatusQueued {
		t.Fatalf("unexpected response: %+v", cr)
	}
}

func TestCheckout_MissingFields(t *testing.T) {
	mock := &mockService{}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/checkout", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueueStatus_Queued(t *testing.T) {
	mock := &mockService{
		queueStatusFn: func(_ context.Context, productID, userID string) (*model.QueueStatusResponse, error) {
			return &model.QueueStatusResponse{
				Position: 5, Status: model.StatusQueued, ProductID: productID, UserID: userID,
			}, nil
		},
	}
	g := setupTest(mock)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/flash-sale/queue-status?product_id=prod-1&user_id=user-1", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var qs model.QueueStatusResponse
	json.Unmarshal(data, &qs)
	if qs.Position != 5 || qs.Status != model.StatusQueued {
		t.Fatalf("unexpected queue status: %+v", qs)
	}
}

func TestQueueStatus_MissingParams(t *testing.T) {
	mock := &mockService{}
	g := setupTest(mock)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/flash-sale/queue-status", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDryRun_WouldSucceed(t *testing.T) {
	mock := &mockService{
		dryRunFn: func(_ context.Context, _ model.CheckoutRequest) (*model.DryRunResponse, error) {
			return &model.DryRunResponse{
				AttestationPassed: true,
				RateLimitPassed:   true,
				StockAvailable:    true,
				WouldSucceed:      true,
				RemainingStock:    42,
				LatencyMs:         5,
			}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/dry-run", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1, IdempotencyKey: "idem-dry-1",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(resp.Data)
	var dr model.DryRunResponse
	json.Unmarshal(data, &dr)
	if !dr.WouldSucceed {
		t.Fatalf("expected dry-run to succeed, got %+v", dr)
	}
	if dr.RemainingStock != 42 {
		t.Fatalf("expected remaining_stock=42, got %d", dr.RemainingStock)
	}
}

func TestDryRun_FailsAtAttestation(t *testing.T) {
	mock := &mockService{
		dryRunFn: func(_ context.Context, _ model.CheckoutRequest) (*model.DryRunResponse, error) {
			return &model.DryRunResponse{
				FailureAt:  "attestation",
				LatencyMs:  1,
			}, nil
		},
	}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/dry-run", model.CheckoutRequest{
		ProductID: "prod-1", UserID: "user-1", DeviceFP: "fp-abc",
		Attestation: "bad", ExpiresAt: 2000000000, Quantity: 1, IdempotencyKey: "idem-dry-2",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var dr model.DryRunResponse
	json.Unmarshal(data, &dr)
	if dr.WouldSucceed {
		t.Fatal("expected dry-run to fail")
	}
	if dr.FailureAt != "attestation" {
		t.Fatalf("expected failure_at=attestation, got %s", dr.FailureAt)
	}
}

func TestDryRun_MissingFields(t *testing.T) {
	mock := &mockService{}
	g := setupTest(mock)

	w := postJSON(t, g, "/flash-sale/dry-run", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMetrics(t *testing.T) {
	mock := &mockService{}
	g := setupTest(mock)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/flash-sale/metrics", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// Snapshot should be a map (possibly empty in tests)
	if resp.Data == nil {
		t.Fatal("expected non-nil metrics data")
	}
}

func TestHealth(t *testing.T) {
	mock := &mockService{}
	g := setupTest(mock)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/flash-sale/health", nil)
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(resp.Data)
	var health map[string]interface{}
	json.Unmarshal(data, &health)
	if health["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", health["status"])
	}
	if health["circuit_breaker"] != "closed" {
		t.Fatalf("expected circuit_breaker=closed (nil CB), got %v", health["circuit_breaker"])
	}
}
