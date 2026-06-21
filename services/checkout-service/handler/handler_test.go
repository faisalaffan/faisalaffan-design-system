package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func setupTest(t *testing.T) (*gin.Engine, *service.CheckoutService, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	outbox := make(chan model.OutboxEvent, 10)
	svc := service.NewCheckoutService(outbox)

	// Use a known HMAC secret for tests.
	os.Setenv("WEBHOOK_SECRET", "test-secret")

	redisRepo := repository.NewRedisClient(nil) // nil = no Redis, no-op
	h := NewHandler(svc, redisRepo)

	r := kit.NewServer(kit.Config{Env: "test"})
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)

	return r, svc, "test-secret"
}

func signBody(t *testing.T, body []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ─── POST /checkout ──────────────────────────────────────────────────────────

func TestCheckout_Success(t *testing.T) {
	r, _, _ := setupTest(t)

	req := model.CheckoutRequest{
		UserID: "user-1",
		Items: []model.OrderItem{
			{ProductID: "prod-1", Name: "Item A", Quantity: 2, UnitPrice: 5000},
			{ProductID: "prod-2", Name: "Item B", Quantity: 1, UnitPrice: 15000},
		},
		Payment: model.PaymentInfo{
			Method:   "card",
			Currency: "IDR",
		},
	}
	body, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/checkout", bytes.NewReader(body))
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Idempotency-Key", uuid.New().String())

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		// The JSON may decode into map[string]interface{}
		// or directly match checkoutResponse — we check string fields.
		raw, _ := json.Marshal(resp.Data)
		json.Unmarshal(raw, &data)
	}
	if data["order_id"] == "" || data["order_id"] == nil {
		t.Fatal("expected non-empty order_id")
	}
	if data["status"] != "confirmed" {
		t.Fatalf("expected status 'confirmed', got %v", data["status"])
	}
}

func TestCheckout_MissingIdempotencyKey(t *testing.T) {
	r, _, _ := setupTest(t)

	req := model.CheckoutRequest{
		UserID: "user-1",
		Items: []model.OrderItem{
			{ProductID: "prod-1", Name: "Item A", Quantity: 1, UnitPrice: 1000},
		},
		Payment: model.PaymentInfo{Currency: "IDR"},
	}
	body, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/checkout", bytes.NewReader(body))
	reqHTTP.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCheckout_InvalidBody(t *testing.T) {
	r, _, _ := setupTest(t)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/checkout", bytes.NewReader([]byte(`{invalid`)))
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Idempotency-Key", uuid.New().String())

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── POST /webhook/payment ───────────────────────────────────────────────────

func TestWebhookPayment_Success(t *testing.T) {
	r, svc, secret := setupTest(t)

	// First create an order so it exists.
	result, err := svc.NewOrder(&model.SagaContext{
		UserID:   "user-1",
		Items:    []model.OrderItem{{ProductID: "p1", Name: "X", Quantity: 1, UnitPrice: 1000}},
		Currency: "IDR",
	})
	if err != nil || !result.Success {
		t.Fatalf("expected saga success, got err=%v success=%v", err, result.Success)
	}

	payload := model.WebhookPayload{
		TransactionID: fmt.Sprintf("txn_%s", result.OrderID),
		OrderID:       result.OrderID,
		Status:        "succeeded",
		Amount:        2000,
		Currency:      "IDR",
	}
	body, _ := json.Marshal(payload)
	sig := signBody(t, body, secret)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/webhook/payment", bytes.NewReader(body))
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("X-Signature", sig)

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the order can be fetched.
	w2 := httptest.NewRecorder()
	reqGet, _ := http.NewRequest("GET", "/api/v1/orders/"+result.OrderID, nil)
	r.ServeHTTP(w2, reqGet)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestWebhookPayment_MissingSignature(t *testing.T) {
	r, _, _ := setupTest(t)

	payload := model.WebhookPayload{
		TransactionID: "txn-1",
		OrderID:       "order-1",
		Status:        "succeeded",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/webhook/payment", bytes.NewReader(body))
	reqHTTP.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookPayment_InvalidSignature(t *testing.T) {
	r, _, _ := setupTest(t)

	payload := model.WebhookPayload{
		TransactionID: "txn-1",
		OrderID:       "order-1",
		Status:        "succeeded",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("POST", "/api/v1/webhook/payment", bytes.NewReader(body))
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("X-Signature", "invalid-signature")

	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── GET /orders/:id ─────────────────────────────────────────────────────────

func TestGetOrder_NotFound(t *testing.T) {
	r, _, _ := setupTest(t)

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("GET", "/api/v1/orders/non-existent", nil)
	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetOrder_Success(t *testing.T) {
	r, svc, _ := setupTest(t)

	result, _ := svc.NewOrder(&model.SagaContext{
		UserID:   "user-get",
		Items:    []model.OrderItem{{ProductID: "p1", Name: "Test", Quantity: 1, UnitPrice: 5000}},
		Currency: "USD",
	})
	if result == nil || !result.Success {
		t.Fatal("expected successful order creation")
	}

	w := httptest.NewRecorder()
	reqHTTP, _ := http.NewRequest("GET", "/api/v1/orders/"+result.OrderID, nil)
	r.ServeHTTP(w, reqHTTP)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp kit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
}

func TestHMACVerification(t *testing.T) {
	h := Handler{hmacKey: []byte("test-secret")}
	body := []byte(`{"hello":"world"}`)
	sig := signBody(t, body, "test-secret")

	if !h.verifyHMAC(body, sig) {
		t.Fatal("expected valid signature")
	}
	if h.verifyHMAC(body, "wrong") {
		t.Fatal("expected invalid signature")
	}
	if h.verifyHMAC([]byte("other"), sig) {
		t.Fatal("expected invalid signature for different body")
	}
}
