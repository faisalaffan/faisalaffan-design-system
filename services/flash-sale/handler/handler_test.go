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

// mockService implements FlashSaleService for unit tests.
type mockService struct {
	checkoutFn    func(context.Context, model.CheckoutRequest) (*model.CheckoutResponse, error)
	queueStatusFn func(context.Context, string, string) (*model.QueueStatusResponse, error)
}

func (m *mockService) Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error) {
	return m.checkoutFn(ctx, req)
}

func (m *mockService) QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error) {
	return m.queueStatusFn(ctx, productID, userID)
}

func setupTest(mock *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	h := New(mock)
	g.POST("/flash-sale/checkout", h.Checkout)
	g.GET("/flash-sale/queue-status", h.QueueStatus)
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
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1,
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
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1,
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
		Attestation: "tok", ExpiresAt: 2000000000, Quantity: 1,
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
