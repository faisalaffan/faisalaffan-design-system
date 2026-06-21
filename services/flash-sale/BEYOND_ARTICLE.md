# Beyond the Article — Production-Grade Improvements

This document catalogs all features in this flash-sale implementation that go **beyond** what was described in the original [news.faisalaffan.com](https://news.faisalaffan.com/news/engineering/flash-sale-qcommerce-system-design) article. These additions address real-world edge cases discovered through production experience and mobile-first requirements.

---

## 1. Idempotency Key — Double-Submit Prevention

**Article gap:** No mechanism to prevent duplicate checkouts from network retries, double-click, or mobile SDK auto-retry.

**Implementation:** `idempotency_key` is a **required** field in `CheckoutRequest`. The backend uses Redis `SetNX` with a 30-second lock TTL and caches the result for 10 minutes.

```go
// repository/redis.go
func (r *FlashSaleRepo) CheckIdempotency(ctx context.Context, idemKey string) (bool, string, error) {
    key := r.idempotencyKey(idemKey)
    ok, err := r.rdb.SetNX(ctx, key, "locked", 30*time.Second).Result()
    if !ok {
        result, _ := r.rdb.Get(ctx, key+":result").Result()
        return true, result, nil // Conflict — return cached result
    }
    return false, "", nil
}
```

**Why it matters for mobile:** OkHttp (Android) and URLSession (iOS) automatically retry failed requests. Without idempotency, a single user tap during a network hiccup triggers duplicate `POST /checkout` calls. The idempotency guard detects the retry and returns the previously computed result instead of processing again.

**Response on conflict:**
```json
HTTP 409 Conflict
{"status": "idempotency_conflict", "order_id": "ORD-previous-result"}
```

---

## 2. Reservation Lifecycle — Compensating Transaction

**Article gap:** Stock is decremented atomically, but there is no mechanism to return stock if the checkout fails downstream (payment failure, order creation failure, etc.).

**Implementation:** The Lua bucket-decrement script now **creates a reservation** with TTL alongside the stock deduction. The reservation tracks:

```
States: RESERVED → CONFIRMED (checkout success)
                    → RELEASED (checkout cancelled)
                    → EXPIRED   (TTL elapsed, auto-reaper)
```

**Endpoints added:**

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/flash-sale/release` | Compensate failed checkout — returns stock |
| `POST` | `/flash-sale/confirm` | Finalize successful checkout |

**Reservation data (Redis hash):**
```
flash:reservation:RES-abc123
  id: RES-abc123
  product_id: flash-indomie-2026
  user_id: user-xyz
  quantity: 2
  bucket_idx: 3
  status: reserved
  created_at: 1719000000000
  expires_at: 1719000300000
```

**Release script (Lua atomic):**
```lua
-- Check status is "reserved" or "expired"
-- If "released" or "confirmed" → idempotent, return success
-- INCRBY bucket key by quantity
-- INCRBY total key by quantity
-- Set status to "released"
```

This ensures stock is never lost — every reservation is either confirmed, released, or auto-expired.

---

## 3. Reaper — Expired Reservation Cleanup

**Article gap:** No mechanism to handle reservations that outlive their TTL (user closes tab, app crashes, network lost).

**Implementation:** A background goroutine runs every 10 seconds scanning for reservations where `expires_at + grace_period < now`. Expired reservations are atomically released via the same Lua script used by `/release`.

```go
func (s *FlashSaleService) StartBackgroundJobs(ctx context.Context, productID string) {
    go func() {
        ticker := time.NewTicker(model.ReaperInterval) // 10s
        for {
            select {
            case <-ctx.Done(): return
            case <-ticker.C:
                n, _ := s.repo.RunReaper(ctx, productID, model.ReaperGracePeriod) // 15s grace
            }
        }
    }()
}
```

**Grace period reasoning:** A 15-second grace period after the 5-minute TTL accounts for clock skew between the application server and Redis, and gives a last-moment retry a chance to confirm.

---

## 4. SSE Queue Stream — Battery-Efficient Push

**Article gap:** Queue status is only available via polling (`GET /queue-status`). For mobile apps, polling drains battery and stops when the app is backgrounded.

**Implementation:** A Server-Sent Events endpoint pushes position updates via Redis pub/sub.

```
GET /flash-sale/queue-stream?product_id=X&user_id=Y
→ text/event-stream
→ data: {"position": 42, "status": "queued"}
→ data: {"position": 15, "status": "queued"}
→ data: {"position": 0, "status": "admitted"}
```

**Architecture:**
```
Admission Consumer → Redis Pub/Sub (flash:queue:{productID}:{userID})
                  → SSE Handler → Client (push, no polling)
```

**Redis pub/sub key:** `flash:queue:{productID}:{userID}` — one channel per user for targeted push.

The SSE connection stays open. When the admission consumer processes the user from the waiting room, it publishes an event. The SSE handler receives it and pushes to the client immediately. No polling overhead.

---

## 5. Waiting Room Cleanup — Stale Entry Removal

**Article gap:** Waiting room entries have no TTL or cleanup mechanism. Users who close their browser tab leave stale entries that inflate queue positions for legitimate waiters.

**Implementation:** A background goroutine runs every 30 seconds removing entries older than `WaitingRoomTTL` (10 minutes).

```go
func (r *FlashSaleRepo) CleanWaitingRoom(ctx context.Context, productID string, ttl time.Duration) (int, error) {
    cutoff := float64(time.Now().Add(-ttl).UnixNano())
    return r.rdb.ZRemRangeByScore(ctx, key, "0", strconv.FormatFloat(cutoff, 'f', 0, 64)).Result()
}
```

This ensures queue positions are accurate — users don't see "Position 200" when 150 of those entries are stale.

---

## 6. Admission Consumer — Active Queue Processing

**Article gap:** The article describes `AdmitNext()` but never wires it into a running loop.

**Implementation:** A goroutine runs on `AdmissionInterval` (500ms) calling `ZPopMin` to admit batches of `AdmissionBatchSize` (10) users from the waiting room. Admitted users get published via Redis pub/sub to trigger SSE notifications.

```go
go func() {
    ticker := time.NewTicker(model.AdmissionInterval) // 500ms
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            users, _ := s.repo.AdmitNext(ctx, productID, model.AdmissionBatchSize)
            for _, userID := range users {
                s.repo.PublishQueueEvent(ctx, productID, userID, model.QueueEvent{
                    Position: 0, Status: "admitted",
                })
            }
        }
    }
}()
```

---

## 7. CDN Cache Headers — Landing Page Protection

**Article gap:** The article describes CDN cache strategy conceptually but provides no middleware implementation.

**Implementation:** Reusable `CDNCache` Gin middleware in `pkg/kit/middleware/cache.go`. Applied globally to all GET/HEAD 2xx/3xx responses. The token endpoint specifically uses it:

```go
r.GET("/flash-sale/token", middleware.CDNCache(middleware.DefaultCacheConfig()), h.Token)
```

**Headers set:**
```
Cache-Control: public, max-age=30, stale-while-revalidate=300, stale-if-error=3600
Surrogate-Control: max-age=30, stale-while-revalidate=300
CDN-Cache-Control: max-age=30
```

This protects the origin server from the 50,000x traffic spike on static product pages while ensuring freshness.

---

## 8. Device Fingerprint Extraction Chain

**Article gap:** The article mentions device fingerprint in the rate limiter but doesn't implement the **extraction chain** with fallback priority.

**Implementation:** `services/flash-sale/fingerprint/extractor.go` implements a multi-tier extraction:

| Priority | Source | Format |
|----------|--------|--------|
| 1 (highest) | `X-Device-Fingerprint` header | `client:<value>` |
| 2 (fallback) | User-Agent + Accept-Language + Sec-CH-UA-Platform + Sec-CH-UA-Model + RemoteAddr | `server:<sha256>` |

The fallback chain is particularly important for web clients that don't run JavaScript (bots, curl, older browsers). The server-side hash is deterministic for the same browser configuration, providing a reasonable fingerprint even without client-side JS.

---

## 9. Event Publisher — Async Order Processing

**Article gap:** The article mentions Kafka conceptually but provides no Go implementation.

**Implementation:** A `Publisher` interface with two implementations:

- **`ChannelPublisher`** — writes JSON to a buffered Go channel (stand-in for Kafka producer). Non-blocking on full buffer — drops event rather than blocking checkout.
- **`LogPublisher`** — writes to structured log (fallback when no message broker).

```go
type OrderCreatedEvent struct {
    OrderID       string `json:"order_id"`
    UserID        string `json:"user_id"`
    ProductID     string `json:"product_id"`
    Quantity      int    `json:"quantity"`
    ReservationID string `json:"reservation_id"`
    DeviceFP      string `json:"device_fp"`
    Timestamp     int64  `json:"timestamp"`
    EventType     string `json:"event_type"`
}
```

The event is published **after** successful checkout (bucket decrement + reservation creation). A consumer goroutine reads from the channel for downstream processing (order service, inventory sync, analytics).

**Non-blocking guarantee:** If the channel is full (100 buffer), the event is logged and dropped — checkout never waits for event delivery.

---

## 10. Token Issuance Endpoint

**Article gap:** The article describes HMAC attestation token verification but doesn't provide a token **issuance** endpoint.

**Implementation:** `GET /flash-sale/token?device_fp=X` generates an HMAC-SHA256 attestation token:

```go
func (h *FlashSaleHandler) Token(c *gin.Context) {
    mac := hmac.New(sha256.New, h.hmacSecret)
    payload := fmt.Sprintf("%s:%d", deviceFP, time.Now().Unix()+30)
    mac.Write([]byte(payload))
    token := hex.EncodeToString(mac.Sum(nil))
    kit.OK(c, gin.H{"token": token, "expires_in": 30})
}
```

**Design:** The HMAC secret is **never** embedded in client code. The client requests a token from this endpoint, then includes it in `POST /checkout`. The server verifies the token by re-computing the HMAC locally (shared secret). This avoids a separate attestation service call on the critical checkout path — trading separation-of-concerns for ~1ms latency savings.

---

## 11. Production-Ready Service Configuration

**Article gap:** Hardcoded port number, no env-based configuration.

**Implementation:** Full `.env.local` support via `pkg/kit/config.go` using `godotenv`. The flash-sale service reads:

```
PORT_FLASH_SALE=8102
REDIS_ADDR=localhost:6379
HMAC_SECRET=dev-secret-do-not-use-in-production
```

All configurable: bucket count, TTL durations, rate limit window, admission interval, reaper frequency.

---

## Summary: Article vs. This Implementation

| Feature | Article | Our Implementation |
|---------|:-------:|:------------------:|
| Stock buckets (10) + Lua atomic | ✅ | ✅ |
| Waiting room (sorted set) | ✅ | ✅ |
| Sliding window rate limit | ✅ | ✅ |
| HMAC attestation | ✅ | ✅ |
| CDN cache strategy | ✅ | ✅ (middleware) |
| **Idempotency key** | ❌ | ✅ SetNX lock + cached result |
| **Reservation lifecycle** | ❌ | ✅ RESERVED → CONFIRMED/RELEASED/EXPIRED |
| **Reaper (auto-release)** | ❌ | ✅ 10s interval, 15s grace |
| **SSE queue stream** | ❌ | ✅ Redis pub/sub push |
| **Waiting room cleanup** | ❌ | ✅ 10min TTL, 30s cleanup |
| **Admission consumer loop** | ❌ | ✅ 500ms interval, batch 10 |
| **Device FP extraction chain** | ❌ | ✅ 2-tier fallback (client → server) |
| **Event publisher** | ❌ | ✅ Channel + log implementations |
| **Token issuance endpoint** | ❌ | ✅ GET /flash-sale/token |
| **Mobile-first design** | ❌ | ✅ SSE, idempotency, deep links, Keychain/Keystore |
| **Env-based configuration** | ❌ | ✅ .env.local via godotenv |

**Result:** 100% article coverage + 11 production-grade improvements.
