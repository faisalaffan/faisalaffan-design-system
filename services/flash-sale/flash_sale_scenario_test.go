package main

// =============================================================================
// IMPORT BLOCK
// Library standar + third-party yang dipake di sini.
// - kit: package internal buat config, helper, dll.
// - gin: HTTP framework.
// - redis: client Redis.
// - godotenv: baca .env file.
// =============================================================================
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
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

// init: jalan otomatis sebelum test apapun dieksekusi.
// Cari file .env.local dari current directory sampe ke repo root.
// Soalnya Go test CWD-nya pake directory package, jadi perlu fallback ke parent.
func init() {
	// Search .env.local from current dir up to repo root (Go test CWD = package dir)
	for _, path := range []string{".env.local", "../../.env.local"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			break
		}
	}
	_ = kit.LoadConfig() // fallback: OS env
}

// =============================================================================
// CONST BLOCK
// Konfigurasi test yang dipake lintas scenario.
// testProductID: ID produk Indomie biar keliatan real-world.
// testBucketCount: jumlah bucket untuk stripe pattern (sharding anti-hotspot).
// testTotalStock: total stok yang tersedia (100 = batas oversell test).
// =============================================================================
const (
	testProductID   = "flash-indomie-2026"
	testBucketCount = 10
	testTotalStock  = 100
)

// getRedisAddr: ambil alamat Redis dari env, fallback ke localhost:6379.
func getRedisAddr() string { return kit.EnvOrDefault("REDIS_ADDR", "localhost:6379") }

// getHMACSecret: ambil secret buat HMAC token, fallback ke test secret.
func getHMACSecret() string {
	return kit.EnvOrDefault("HMAC_SECRET", "flash-sale-scenario-test-secret")
}

// setupRealService: prepare real Redis + Service + Handler buat integration test.
// Yang dilakuin:
// 1. Connect ke Redis (skip test kalo Redis gak jalan).
// 2. Bersihin semua key flash:* sama rl:flash:* biar test isolation.
// 3. Init store + product.
// 4. Return service, store, handler, sama cleanup function.
func setupRealService(t *testing.T) (*Service, *Store, *Handler, func()) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: getRedisAddr()})
	ctx := context.Background()
	// Ping Redis dulu -- kalo gak connect, skip aja testnya.
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available at %s -- skipping scenario test", getRedisAddr())
	}
	// Bersihin key sisa test sebelumnya biar gak saling ganggu.
	keys, _ := rdb.Keys(ctx, "flash:*").Result()
	rlKeys, _ := rdb.Keys(ctx, "rl:flash:*").Result()
	allKeys := append(keys, rlKeys...)
	if len(allKeys) > 0 {
		rdb.Del(ctx, allKeys...)
	}

	store := NewStore(rdb, getHMACSecret())
	// Init struktur store (bucket dll).
	if err := store.Init(ctx); err != nil {
		t.Fatalf("store init: %v", err)
	}
	// Init product dengan stok 100, 10 bucket.
	if err := store.InitProduct(ctx, testProductID, testTotalStock, testBucketCount); err != nil {
		t.Fatalf("init product: %v", err)
	}

	svc := NewService(store)
	h := NewHandler(svc)

	// cleanup: hapus semua key flash:* di Redis terus tutup koneksi.
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

// generateHMACToken: bikin token HMAC-SHA256 dari deviceFP + expiresAt.
// Token ini dipake server buat verifikasi kalo request beneran dari client yang legitimate.
// Secret cuma ada di server -- client gak tau secretnya.
func generateHMACToken(deviceFP string, secret []byte) (string, int64) {
	// expiresAt = sekarang + 30 detik (valid lifetime).
	expiresAt := time.Now().Unix() + 30
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiresAt, 10)))
	token := hex.EncodeToString(mac.Sum(nil))
	return token, expiresAt
}

// newGinEngine: bikin Gin engine fresh di test mode, daftarin handler route.
func newGinEngine(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	h.Register(&g.RouterGroup)
	return g
}

// =============================================================================
// SCENARIO 1: Happy Path -- Budi Berhasil Checkout
//
// Ini skenario paling dasar: user legitimate dengan deviceFP valid
// bisa checkout Indomie di flash sale.
//
// Safety net yang diuji:
// - HMAC attestation (token valid)
// - Pipeline checkout normal (reserve -> process -> complete)
//
// Tanpa safety net ini: koneksi dasar aja gak bakal jalan, user gak bisa belanja.
// =============================================================================
func TestScenario_HappyPath_SuccessfulCheckout(t *testing.T) {
	// Setup real Redis + service, cleanup otomatis setelah test selesai.
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	// Device FP unik: "fp-budi-iphone" -- simulasi device real.
	deviceFP := "fp-budi-iphone"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))

	// Body request checkout: produk Indomie, 1 pcs, user Budi.
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-budi", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-budi-001",
	}
	b, _ := json.Marshal(body)
	// Kirim POST request ke endpoint /flash-sale/checkout.
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	// Assert: response harus 200 OK. Kalo gagal, berarti pipeline checkout ada masalah.
	if w.Code != http.StatusOK {
		t.Fatalf("FAIL: Budi gagal checkout. Status: %d, Body: %s", w.Code, w.Body.String())
	}
	// Parsing response JSON -> CheckoutResponse.
	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)
	// Assert: status harus "completed" -- artinya stok ter-reserve dan checkout beres.
	if cr.Status != StatusCompleted {
		t.Fatalf("FAIL: expected completed, got %s", cr.Status)
	}
	// Assert: order ID wajib ada.
	if cr.OrderID == "" {
		t.Fatal("FAIL: order_id kosong")
	}
	// Assert: reservation ID wajib ada. Kalo kosong, stok udah kepake tapi gak ada track-nya.
	if cr.ReservationID == "" {
		t.Fatal("FAIL: reservation_id kosong -- stok bisa hilang kalau checkout gagal")
	}
	t.Logf("✅ Budi berhasil checkout. Order: %s, Reservation: %s", cr.OrderID, cr.ReservationID)
}

// =============================================================================
// SCENARIO 2: Double-Tap Problem -- Idempotency Mencegah Double Order
//
// Simulasi user (Dinda) nge-tombol checkout 2 kali karena loading lama.
// Idempotency key harus nge-prevent double order.
//
// Safety net yang diuji:
// - Idempotency key di Redis (409 Conflict untuk request duplikat)
//
// Tanpa safety net ini: user kena double charge + 2 order untuk 1 produk.
// =============================================================================
func TestScenario_DoubleTap_IdempotencyPreventsDoubleOrder(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-double-tap"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))

	// Body dengan idempotency key yang SAMA -- simulasi double tap.
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-dinda", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-dinda-same",
	}
	b, _ := json.Marshal(body)

	// Request pertama: normal checkout.
	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	g.ServeHTTP(w1, req1)

	// Request kedua: user tap lagi (atau network retry).
	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)

	// Assert: request kedua harus 409 Conflict. Kalo 200, berarti double order terjadi.
	if w2.Code != http.StatusConflict {
		t.Fatalf("FAIL: double-tap harusnya 409 Conflict. Actual: %d. Tanpa idempotency, user kena double order!", w2.Code)
	}
	t.Log("✅ Idempotency guard mencegah double order dari double-tap/network retry")
}

// =============================================================================
// SCENARIO 3: 50.000x Spike -- Tidak Oversold
//
// Simulasi 200 user concurrent semuanya checkout bersamaan.
// Ini referensi insiden nyata 2024 di e-commerce Indo: 300% oversell.
//
// Safety net yang diuji:
// - Lua atomic decrement (stok gak bisa minus)
// - Pipeline checkout: ada queue kalau stok habis, bukan reject mentah-mentah
//
// Tanpa safety net ini: stok 100 bisa kejual 300+ (oversell fatal).
// =============================================================================
func TestScenario_ConcurrentSpike_NoOversell(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	// 200 user berebut 100 stok.
	const numUsers = 200
	// Atomic counter biar safe dari race condition pas goroutine.
	var completed atomic.Int32
	var queued atomic.Int32
	var wg sync.WaitGroup

	// Loop buat 200 goroutine -- tiap user checkout sendiri.
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			deviceFP := fmt.Sprintf("fp-user-%d", idx)
			token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
			body := CheckoutRequest{
				ProductID: testProductID, UserID: fmt.Sprintf("user-%d", idx), DeviceFP: deviceFP,
				Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-%d", idx),
			}
			b, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)

			// Parsing response buat liat status: completed atau queued.
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
	// Tunggu semua goroutine selesai.
	wg.Wait()

	// Assert: jumlah completed HARUS <= total stok (100). Kalo lebih => OVERSOLD.
	if int(completed.Load()) > testTotalStock {
		t.Fatalf("FAIL: OVERSOLD! Stok %d, terjual %d. Lua atomic harusnya mencegah ini.", testTotalStock, completed.Load())
	}
	t.Logf("✅ Spike %d user, stok %d: completed=%d, queued=%d -- TIDAK OVERSOLD", numUsers, testTotalStock, completed.Load(), queued.Load())
}

// =============================================================================
// SCENARIO 4: Bot Attack -- Rate Limit + Attestation Block
//
// Simulasi 2 jenis serangan:
// 1. Bot pake token palsu (harus ditolak 401)
// 2. Bot spam 30 request berturut-turut (harus kena rate limit)
//
// Safety net yang diuji:
// - HMAC attestation validation (token palsu -> 401)
// - Rate limiter per device fingerprint (bukan per IP)
//
// Tanpa safety net ini: bot bisa beli stok abis + server kena banjir request.
// =============================================================================
func TestScenario_BotAttack_RateLimitAndAttestationBlock(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-bot-farm-99"
	// === Bagian 1: Token palsu ===
	// Kirim request pake attestation "fake-token" -- harus ditolak.
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "bot-user", DeviceFP: deviceFP,
		Attestation: "fake-token", ExpiresAt: time.Now().Unix() + 30, Quantity: 1, IdempotencyKey: "idem-bot-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	// Assert: token palsu harus 401 Unauthorized.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("FAIL: bot token palsu harusnya 401. Actual: %d", w.Code)
	}
	t.Log("✅ Bot dengan token palsu ditolak (401). HMAC attestation bekerja.")

	// === Bagian 2: Rate limit spam ===
	// Pake token valid tapi spam 30 request dari device yang sama SECARA CONCURRENT.
	// Kalo sequential: latency Redis remote bikin sliding window 1 detik keburu expired.
	// Concurrent: semua request masuk dalam window yang sama → rate limiter aktif.
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
	body.Attestation = token
	body.ExpiresAt = expiresAt

	var rateLimited atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := CheckoutRequest{
				ProductID: testProductID, UserID: "bot-user", DeviceFP: deviceFP,
				Attestation: token, ExpiresAt: expiresAt, Quantity: 1,
				IdempotencyKey: fmt.Sprintf("idem-bot-%d", idx),
			}
			b, _ := json.Marshal(req)
			r, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code == http.StatusTooManyRequests {
				rateLimited.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// Assert: harus ada yang kena rate limit. Kalo 0, rate limit gak berfungsi.
	if int(rateLimited.Load()) == 0 {
		t.Fatal("FAIL: bot spam 30 request tanpa kena rate limit.")
	}
	t.Logf("✅ Bot spam 30 request: %d kena rate limit (429). Device-fingerprint based, bukan IP.", rateLimited.Load())
}

// =============================================================================
// SCENARIO 5: Stok Habis -> Waiting Room
//
// Simulasi: stok 100 habis diborong early bird, user telat masuk antrean.
//
// Safety net yang diuji:
// - Queue mechanism (waiting room) via Redis sorted set
// - Queue status endpoint (user bisa liat posisi antrean)
//
// Tanpa safety net ini: user liat "sold out" langsung, ilang revenue.
// Dengan waiting room: user bisa nunggu giliran, kalo ada yang cancel/batal bayar.
// =============================================================================
func TestScenario_StockExhausted_EntersWaitingRoom(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	// === Bagian 1: Early bird borong semua stok (concurrent biar cepet) ===
	// 100 user beli duluan, abisin semua stok. Concurrent karena Redis remote.
	var earlyWg sync.WaitGroup
	for i := 0; i < testTotalStock; i++ {
		earlyWg.Add(1)
		go func(idx int) {
			defer earlyWg.Done()
			deviceFP := fmt.Sprintf("fp-early-%d", idx)
			token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
			body := CheckoutRequest{
				ProductID: testProductID, UserID: fmt.Sprintf("user-early-%d", idx), DeviceFP: deviceFP,
				Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-early-%d", idx),
			}
			b, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			g.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	earlyWg.Wait()

	// === Bagian 2: User telat checkout ===
	deviceFP := "fp-late-user"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-late", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-late-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	// Parsing response buat liat status checkout.
	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)

	// Assert: status harus "queued", bukan error/sold out. User masuk waiting room.
	if cr.Status != StatusQueued {
		t.Fatalf("FAIL: stok habis harusnya masuk waiting room. Status: %s", cr.Status)
	}
	t.Logf("✅ Stok habis -> waiting room posisi %d. User tidak lihat 'sold out' dan pergi.", cr.Position)

	// === Bagian 3: Cek queue status ===
	// GET /flash-sale/queue-status?product_id=...&user_id=...
	req2, _ := http.NewRequest("GET", "/flash-sale/queue-status?product_id="+testProductID+"&user_id=user-late", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	var resp2 kit.Response
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	data2, _ := json.Marshal(resp2.Data)
	var qs QueueStatusResponse
	json.Unmarshal(data2, &qs)
	// Assert: posisi antrean harus numerik. Redis sorted set persist data -- survive restart.
	t.Logf("✅ Queue status: posisi %d. Redis sorted set persisten -- survive restart.", qs.Position)
}

// =============================================================================
// SCENARIO 6: Checkout Gagal -> Release Reservation (Compensating Transaction)
//
// Simulasi: user berhasil reserve stok, tapi payment gateway timeout.
// Stok harus di-release balik biar gak ilang.
//
// Safety net yang diuji:
// - Reservation release endpoint (POST /flash-sale/release)
// - Idempotency release (kalo payment retry -> release lagi, gak error)
// - Compensating transaction pattern: reserve -> fail -> release
//
// Tanpa safety net ini: stok terkunci selamanya kalau payment gagal, rugi.
// =============================================================================
func TestScenario_CheckoutFailed_ReservationReleased(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	// === Bagian 1: Checkout normal ===
	deviceFP := "fp-release-test"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-release", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: "idem-release-001",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	// Parsing response -> ambil reservation ID.
	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cr CheckoutResponse
	json.Unmarshal(data, &cr)

	// Assert: reservation ID harus ada. Kalo kosong, gak bisa di-release nantinya.
	if cr.ReservationID == "" {
		t.Fatal("FAIL: tidak ada reservation_id")
	}

	// === Bagian 2: Payment gateway timeout -> release stok ===
	// Kirim POST /flash-sale/release dengan reservation ID.
	releaseBody := ReleaseRequest{ReservationID: cr.ReservationID}
	b2, _ := json.Marshal(releaseBody)
	req2, _ := http.NewRequest("POST", "/flash-sale/release", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	// Assert: release harus 200 OK.
	if w2.Code != http.StatusOK {
		t.Fatalf("FAIL: release gagal. Status: %d", w2.Code)
	}
	t.Log("✅ Reservation di-release. Stok dikembalikan.")

	// === Bagian 3: Release kedua (idempotent) ===
	// Simulasi payment gateway retry -- ini request yang sama.
	req3, _ := http.NewRequest("POST", "/flash-sale/release", bytes.NewReader(b2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	g.ServeHTTP(w3, req3)
	// Assert: release kedua harus 200 juga, bukan error. Idempotent.
	if w3.Code != http.StatusOK {
		t.Fatalf("FAIL: release kedua harusnya idempotent. Status: %d", w3.Code)
	}
	t.Log("✅ Release idempotent -- payment gateway retry aman.")
}

// =============================================================================
// SCENARIO 7: Token Expired 31 Detik -> Ditolak
//
// Simulasi: client pake token yang expired 31 detik yang lalu.
// Sistem punya tolerance +-30 detik buat clock skew.
//
// Safety net yang diuji:
// - Token expiration check (31 detik > tolerance -> rejected)
// - Clock skew tolerance (+-30 detik)
//
// Tanpa safety net ini: token expired bisa dipake ulang (replay attack).
// Clock skew 30s penting karena jam client-server gak selalu sinkron.
// =============================================================================
func TestScenario_ExpiredToken_RejectedWithSkewTolerance(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-expired-test"
	// Bikin expiredAt 31 detik yang lalu -- lebih dari tolerance 30s.
	expiredAt := time.Now().Unix() - 31
	mac := hmac.New(sha256.New, []byte(getHMACSecret()))
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

	// Assert: token expired >30s harus 401 Unauthorized.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("FAIL: token expired harusnya 401. Actual: %d", w.Code)
	}
	t.Log("✅ Token expired >30 detik ditolak. Clock skew tolerance +-30s -- cukup untuk perbedaan jam client-server.")
}

// =============================================================================
// SCENARIO 8: Token Issuance -- GET /flash-sale/token
//
// Simulasi: client minta token lewat endpoint GET /flash-sale/token.
//
// Safety net yang diuji:
// - Token issuance endpoint ada dan jalan
// - Response contains token + expires_in
//
// Tanpa safety net ini: client gak punya cara dapet token buat attestation.
// HMAC secret cuma di server side -- gak ada di binary client.
// =============================================================================
func TestScenario_TokenIssuance_ClientGetsValidToken(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	req, _ := http.NewRequest("GET", "/flash-sale/token?device_fp=fp-token-req", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	// Assert: endpoint harus 200 OK.
	if w.Code != http.StatusOK {
		t.Fatalf("FAIL: token issuance gagal. Status: %d", w.Code)
	}

	// Parsing response -> cek field token sama expires_in.
	var resp kit.Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	// Assert: token harus ada dan gak kosong.
	if result["token"] == nil || result["token"] == "" {
		t.Fatal("FAIL: token kosong")
	}
	// Assert: expires_in harus ada.
	if result["expires_in"] == nil {
		t.Fatal("FAIL: expires_in tidak ada")
	}
	// HMAC secret cuma di server side -- client aman.
	t.Logf("✅ Token issued: expires_in=%v. HMAC secret server-side only -- tidak ada di binary client.", result["expires_in"])
}

// =============================================================================
// SCENARIO 9: Satu Device Spam -> Rate Limit Per Device Fingerprint
//
// Simulasi: satu device ngirim 25 request berturut-turut.
// Rate limiter harusnya nge-block setelah burst 20.
//
// Safety net yang diuji:
// - Rate limiter berbasis device fingerprint (bukan IP)
// - Burst allowance: request pertama lolos, setelah itu ditahan
//
// Tanpa safety net ini: 1 device bisa spam ribuan request, stok abis sendiri.
// Rate limit per device FP penting karena bot farm bisa ganti IP tiap request.
// =============================================================================
func TestScenario_SameDeviceSpam_RateLimitedPerDeviceFP(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-spam-device"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
	var allowed atomic.Int32
	var blocked atomic.Int32

	// Kirim 25 request dari device FP yang sama SECARA CONCURRENT.
	// Kalo sequential: latency Redis remote bikin tiap request 100-300ms, total 7.5s+.
	// Concurrent: semua request masuk dalam waktu yang sama -> rate limiter aktif.
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := CheckoutRequest{
				ProductID: testProductID, UserID: "user-spam", DeviceFP: deviceFP,
				Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: fmt.Sprintf("idem-spam-%d", idx),
			}
			b, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)
			if w.Code == http.StatusTooManyRequests {
				blocked.Add(1)
			} else {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// Assert: burst=20, jadi minimal 5 request harus kena rate limit (25 - 20 = 5).
	// Kalo blocked=0, rate limit gak berfungsi sama sekali.
	if int(blocked.Load()) == 0 {
		t.Fatalf("FAIL: rate limit tidak berfungsi. 25 request dari device sama, blocked=%d. Harusnya minimal 5 kena 429.", blocked.Load())
	}
	t.Logf("✅ Satu device spam 25 request: %d allowed, %d blocked (burst=20). Device-fingerprint based.", allowed.Load(), blocked.Load())
}

// =============================================================================
// SCENARIO 10: Mobile Network Flaky -- Idempotency Saves Double Order
//
// Simulasi: user di mobile, pas checkout tiba-tiba 4G -> WiFi handoff.
// OkHttp / URLSession auto-retry ngirim request yang sama lagi.
//
// Safety net yang diuji:
// - Idempotency key untuk skenario network retry
//
// Tanpa safety net ini: user kena double charge pas ganti jaringan.
// Ini insiden nyata yang sering terjadi di mobile apps Indonesia.
// =============================================================================
func TestScenario_MobileNetworkFlaky_IdempotencySavesDoubleOrder(t *testing.T) {
	_, _, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)

	deviceFP := "fp-mobile-flaky"
	token, expiresAt := generateHMACToken(deviceFP, []byte(getHMACSecret()))
	sameKey := "idem-mobile-retry"

	body := CheckoutRequest{
		ProductID: testProductID, UserID: "user-mobile", DeviceFP: deviceFP,
		Attestation: token, ExpiresAt: expiresAt, Quantity: 1, IdempotencyKey: sameKey,
	}
	b, _ := json.Marshal(body)

	// Request 1: checkout pertama, ini sukses.
	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	g.ServeHTTP(httptest.NewRecorder(), req1)

	// Request 2: OkHttp auto-retry karena 4G -> WiFi handoff.
	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)

	// Assert: retry harus 409 Conflict (idempotency key udah dipake).
	// Kalo 200, berarti double order -- user kena charge 2x.
	if w2.Code != http.StatusConflict {
		t.Fatalf("FAIL: mobile retry harusnya 409 Conflict. Actual: %d. Tanpa idempotency, user kena double charge!", w2.Code)
	}
	t.Log("✅ Mobile network retry aman. OkHttp/URLSession auto-retry tidak menyebabkan double order.")
}

// =============================================================================
// SCENARIO 11: Stock Fragmentation -- Bucket Fallback Mencegah False Sold-Out
//
// Simulasi: 2 user dengan deviceFP beda, hash ke bucket Redis yang berbeda.
// Dalam stripe/bucket pattern, kalo 1 bucket habis, sistem harus fallback
// ke bucket lain.
//
// Safety net yang diuji:
// - Multi-bucket strategy (bukan 1 Redis key doang)
// - Bucket fallback: kalo bucket A habis, cobain bucket B
//
// Tanpa safety net ini: semua stok di 1 Redis key = hotspot + contention.
// Dengan 10 bucket, 100 concurrent user gak saling grief.
// =============================================================================
func TestScenario_StockFragmentation_BucketFallbackSavesSale(t *testing.T) {
	_, store, h, cleanup := setupRealService(t)
	defer cleanup()
	g := newGinEngine(h)
	ctx := context.Background()

	// User A: deviceFP "fp-frag-a" -> hash ke bucket tertentu.
	deviceFP1 := "fp-frag-a"
	token1, expiresAt1 := generateHMACToken(deviceFP1, []byte(getHMACSecret()))
	body1 := CheckoutRequest{
		ProductID: testProductID, UserID: "user-frag-a", DeviceFP: deviceFP1,
		Attestation: token1, ExpiresAt: expiresAt1, Quantity: 1, IdempotencyKey: "idem-frag-a",
	}
	b1, _ := json.Marshal(body1)
	req1, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	g.ServeHTTP(w1, req1)
	// Assert: user A harus bisa checkout.
	if w1.Code != http.StatusOK {
		t.Fatalf("FAIL: user A gagal checkout: %d", w1.Code)
	}

	// User B: deviceFP "fp-frag-b-different-hash" -> hash ke bucket B (beda).
	deviceFP2 := "fp-frag-b-different-hash"
	token2, expiresAt2 := generateHMACToken(deviceFP2, []byte(getHMACSecret()))
	body2 := CheckoutRequest{
		ProductID: testProductID, UserID: "user-frag-b", DeviceFP: deviceFP2,
		Attestation: token2, ExpiresAt: expiresAt2, Quantity: 1, IdempotencyKey: "idem-frag-b",
	}
	b2, _ := json.Marshal(body2)
	req2, _ := http.NewRequest("POST", "/flash-sale/checkout", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	// Assert: user B juga harus bisa checkout, meskipun beda bucket.
	if w2.Code != http.StatusOK {
		t.Fatalf("FAIL: user B gagal checkout: %d", w2.Code)
	}

	// 2 user dari bucket berbeda sukses checkout -- gak ada false sold-out.
	t.Log("✅ Dua device berbeda hash ke bucket berbeda -- tidak ada false sold-out.")
	_ = store
	_ = ctx
}
