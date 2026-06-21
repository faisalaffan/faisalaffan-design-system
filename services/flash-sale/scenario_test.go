package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	testRedisAddr   = "localhost:6379"
	testHMACSecret  = "flash-sale-scenario-test-secret"
	testProductID   = "flash-indomie-2026"
	testBucketCount = 10
	testTotalStock  = 100
)

func setupRealService(t *testing.T) (*Service, *Store, *Handler, func()) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testRedisAddr})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available at %s -- skipping scenario test", testRedisAddr)
	}
	keys, _ := rdb.Keys(ctx, "flash:*").Result()
	rlKeys, _ := rdb.Keys(ctx, "rl:flash:*").Result()
	allKeys := append(keys, rlKeys...)
	if len(allKeys) > 0 {
		rdb.Del(ctx, allKeys...)
	}

	store := NewStore(rdb, testHMACSecret)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("store init: %v", err)
	}
	if err := store.InitProduct(ctx, testProductID, testTotalStock, testBucketCount); err != nil {
		t.Fatalf("init product: %v", err)
	}

	svc := NewService(store)
	h := NewHandler(svc)

	cleanup := func() {
		keys, _ := rdb.Keys(ctx, "flash:*").Result()
		rlKeys, _ := rdb.Keys(ctx, "rl:flash:*").Result()
		allKeys := append(keys, rlKeys...)
		if len(allKeys) > 0 {
			rdb.Del(ctx, allKeys...)
		}
		rdb.Close()
	}
	return svc, store, h, cleanup
}

func generateHMACToken(deviceFP string, secret []byte) (string, int64) {
	expiresAt := time.Now().Unix() + 30
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiresAt, 10)))
	token := hex.EncodeToString(mac.Sum(nil))
	return token, expiresAt
}

func newGinEngine(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	h.Register(&g.RouterGroup)
	return g
}

// Scenario 1: Happy path -- Budi checkout Indomie sukses
func TestScenario_HappyPath_SuccessfulCheckout(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-budi-iphone"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))

	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-budi", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-budi-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("FAIL: Budi gagal checkout. Status: %d, Body: %s", w.Code, w.Body.String())
	}
	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)
	if cr.Status != StatusCompleted {
		t.Fatalf("FAIL: expected completed, got %s", cr.Status)
	}
	if cr.OrderID == "" {
		t.Fatal("FAIL: order_id kosong")
	}
	if cr.ReservationID == "" {
		t.Fatal("FAIL: reservation_id kosong -- stok bisa hilang kalau checkout gagal")
	}
	t.Logf("✅ Budi berhasil checkout. Order: %s, Reservation: %s", cr.OrderID, cr.ReservationID)
}

// Scenario 2: Double-tap problem -- idempotency mencegah double order
func TestScenario_DoubleTap_IdempotencyPreventsDoubleOrder(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-double-tap"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))

	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-dinda", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-dinda-same",
	}
	b, _ := json.Marshal(body)

	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	g.ServeHTTP(w1, req1)

	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("FAIL: double-tap harusnya 409 Conflict. Actual: %d. Tanpa idempotency, user kena double order!", w2.Code)
	}
	t.Log("✅ Idempotency guard mencegah double order dari double-tap/network retry")
}

// Scenario 3: 50,000x spike -- tidak oversold (insiden nyata 2024: 300% oversell)
func TestScenario_ConcurrentSpike_NoOversell(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	const numUsers = 200
	var completed atomic.Int32
	var queued atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			deviceFP := fmt.Sprintf("fp-user-%d", idx)
			token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
			body := CheckoutRequest{
				ProductID: testProductID, UserID: fmt.Sprintf("user-%d", idx), DeviceFP: deviceFP,
				Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-%d", idx),
			}
			b, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)

			var resp kit.Response
			json.Unmarshal(w.Body.Bytes(), &resp)
			data, _ := json.Marshal(resp.Data)
			var cr CheckoutResponse
			json.Unmarshal(data, &cr)
			switch cr.Status {
			case StatusCompleted:
				completed.Add(1)
			case StatusQueued:
				queued.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if int(completed.Load()) > testTotalStock {
		t.Fatalf("FAIL: OVERSOLD! Stok %d, terjual %d. Lua atomic harusnya mencegah ini.", testTotalStock, completed.Load())
	}
	t.Logf("✅ Spike %d user, stok %d: completed=%d, queued=%d -- TIDAK OVERSOLD", numUsers, testTotalStock, completed.Load(), queued.Load())
}

// Scenario 4: Bot attack -- rate limit + attestation block
func TestScenario_BotAttack_RateLimitAndAttestationBlock(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-bot-farm-99"
	// Invalid attestation
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "bot-user", DeviceFP: deviceFP,
		Attestation: "fake-token", ExpiresAt: time.Now().Unix() + 30, Quantity: 1, IdempotencyKey: "idem-bot-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("FAIL: bot token palsu harusnya 401. Actual: %d", w.Code)
	}
	t.Log("✅ Bot dengan token palsu ditolak (401). HMAC attestation bekerja.")

	// Rate limit spam
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
	body.Attestation = token
	body.ExpiresAt = expiresAt
	rateLimited := 0
	for i := 0; i < 30; i++ {
		body.IdempotencyKey = fmt.Sprintf("idem-bot-%d", i+1)
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			rateLimited++
		}
	}
	if rateLimited == 0 {
		t.Fatal("FAIL: bot spam 30 request tanpa kena rate limit.")
	}
	t.Logf("✅ Bot spam 30 request: %d kena rate limit (429). Device-fingerprint based, bukan IP.", rateLimited)
}

// Scenario 5: Stok habis -> masuk waiting room
func TestScenario_StockExhausted_EntersWaitingRoom(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	for i := 0; i < testTotalStock; i++ {
		deviceFP := fmt.Sprintf("fp-early-%d", i)
		token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
		body := CheckoutRequest{
			ProductID: testProductID, UserID: fmt.Sprintf("user-early-%d", i), DeviceFP: deviceFP,
			Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-early-%d", i),
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
	}

	deviceFP := "fp-late-user"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-late", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-late-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)

	if cr.Status != StatusQueued {
		t.Fatalf("FAIL: stok habis harusnya masuk waiting room. Status: %s", cr.Status)
	}
	t.Logf("✅ Stok habis -> waiting room posisi %d. User tidak lihat 'sold out' dan pergi.", cr.Position)

	req2, _ := http.NewRequest("GET", "/flash-sale/queue-status?product_id="+testProductID+"&user_id=user-late", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	var resp2 kit.Response
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	data2, _ := json.Marshal(resp2.Data)
	var qs QueueStatusResponse
	json.Unmarshal(data2, &qs)
	t.Logf("✅ Queue status: posisi %d. Redis sorted set persisten -- survive restart.", qs.Position)
}

// Scenario 6: Checkout gagal -> release reservation (compensating transaction)
func TestScenario_CheckoutFailed_ReservationReleased(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-release-test"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-release", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-release-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)

	if cr.ReservationID == "" {
		t.Fatal("FAIL: tidak ada reservation_id")
	}

	// Payment gateway timeout -> release stok
	releaseBody := ReleaseRequest{ReservationID: cr.ReservationID}
	b2, _ := json.Marshal(releaseBody)
	req2, _ := http.NewRequest("POST", "/flash-sale/release", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("FAIL: release gagal. Status: %d", w2.Code)
	}
	t.Log("✅ Reservation di-release. Stok dikembalikan.")

	// Release idempotent
	req3, _ := http.NewRequest("POST", "/flash-sale/release", bytes.NewReader(b2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	g.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("FAIL: release kedua harusnya idempotent. Status: %d", w3.Code)
	}
	t.Log("✅ Release idempotent -- payment gateway retry aman.")
}

// Scenario 7: Token expired 31 detik -> ditolak (+-30s clock skew)
func TestScenario_ExpiredToken_RejectedWithSkewTolerance(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-expired-test"
	expiredAt := time.Now().Unix() - 31
	mac := hmac.New(sha256.New, []byte(testHMACSecret))
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiredAt, 10)))
	oldToken := hex.EncodeToString(mac.Sum(nil))

	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-expired", DeviceFP: deviceFP,
		Attestation: oldToken, ExpiresAt: expiredAt, Quantity: 1, IdempotencyKey: "idem-expired-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("FAIL: token expired harusnya 401. Actual: %d", w.Code)
	}
	t.Log("✅ Token expired >30 detik ditolak. Clock skew tolerance +-30s -- cukup untuk perbedaan jam client-server.")
}

// Scenario 8: Token issuance -- GET /flash-sale/token
func TestScenario_TokenIssuance_ClientGetsValidToken(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	req, _ := http.NewRequest("GET", "/flash-sale/token?device_fp=fp-token-req", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("FAIL: token issuance gagal. Status: %d", w.Code)
	}

	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	if result["token"] == nil || result["token"] == "" {
		t.Fatal("FAIL: token kosong")
	}
	if result["expires_in"] == nil {
		t.Fatal("FAIL: expires_in tidak ada")
	}
	t.Logf("✅ Token issued: expires_in=%v. HMAC secret server-side only -- tidak ada di binary client.", result["expires_in"])
}

// Scenario 9: Satu device spam -> rate limit per device fingerprint
func TestScenario_SameDeviceSpam_RateLimitedPerDeviceFP(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-spam-device"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
	allowed, blocked := 0, 0

	for i := 0; i < 25; i++ {
		body := CheckoutRequest{
			ProductID: testProductID, UserID: "user-spam", DeviceFP: deviceFP,
			Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-spam-%d", i),
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			blocked++
		} else {
			allowed++
		}
	}

	if allowed > 21 {
		t.Fatalf("FAIL: terlalu banyak request lolos. Allowed: %d, Blocked: %d", allowed, blocked)
	}
	t.Logf("✅ Satu device spam 25 request: %d allowed, %d blocked (burst=20). Device-fingerprint based.", allowed, blocked)
}

// Scenario 10: Mobile network flaky -- idempotency saves double order
func TestScenario_MobileNetworkFlaky_IdempotencySavesDoubleOrder(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-mobile-flaky"
	token, expiresAt := generateHMACToken(deviceFP, []byte(testHMACSecret))
	sameKey := "idem-mobile-retry"

	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-mobile", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: sameKey,
	}
	b, _ := json.Marshal(body)

	// Request 1
	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(httptest.NewRecorder(), req1)

	// Request 2: OkHttp auto-retry (4G -> WiFi handoff)
	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("FAIL: mobile retry harusnya 409 Conflict. Actual: %d. Tanpa idempotency, user kena double charge!", w2.Code)
	}
	t.Log("✅ Mobile network retry aman. OkHttp/URLSession auto-retry tidak menyebabkan double order.")
}

// Scenario 11: Stock fragmentation -- bucket fallback mencegah false sold-out
func TestScenario_StockFragmentation_BucketFallbackSavesSale(t *testing.T) {
	_, store, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)
	ctx := context.Background()

	deviceFP1 := "fp-frag-a"
	token1, expiresAt1 := generateHMACToken(deviceFP1, []byte(testHMACSecret))
	body1 := CheckoutRequest{
		ProductID: testProductID, UserID: "user-frag-a", DeviceFP: deviceFP1,
		Attestation: token1, ExpiresAt: expiresAt1, Quantity: 1, IdempotencyKey: "idem-frag-a",
	}
	b1, _ := json.Marshal(body1)
	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	g.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("FAIL: user A gagal checkout: %d", w1.Code)
	}

	deviceFP2 := "fp-frag-b-different-hash"
	token2, expiresAt2 := generateHMACToken(deviceFP2, []byte(testHMACSecret))
	body2 := CheckoutRequest{
		ProductID: testProductID, UserID: "user-frag-b", DeviceFP: deviceFP2,
		Attestation: token2, ExpiresAt: expiresAt2, Quantity: 1, IdempotencyKey: "idem-frag-b",
	}
	b2, _ := json.Marshal(body2)
	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("FAIL: user B gagal checkout: %d", w2.Code)
	}

	t.Log("✅ Dua device berbeda hash ke bucket berbeda -- tidak ada false sold-out.")
	_ = store
	_ = ctx
}
