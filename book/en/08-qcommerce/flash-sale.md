# Flash Sale

High-throughput flash sale checkout engine using Redis Lua bucketed stock, a sorted set waiting room, HMAC-SHA256 attestation tokens, and sliding window rate limiting per device fingerprint.

Port **8103** | Package `flash-sale/`

## Architecture

```mermaid
%%{init: {"theme": "base", "themeVariables": {"background": "#ffffff"}}}%%
sequenceDiagram
    participant U as User
    participant FS as Flash Sale Service
    participant R as Redis
    participant ES as External Services

    U->>FS: Request attestation token
    FS->>U: HMAC token (WebGL+canvas+CAPTCHA)

    U->>FS: POST /checkout (with token)
    FS->>FS: Verify token & rate limit
    FS->>R: Lua: check stock buckets (N=10)
    R-->>FS: Stock available

    FS->>R: ZAdd to waiting room
    Note over FS,R: User in queue
    FS->>U: Queue status (position)

    loop Admit from waiting room
        FS->>R: ZPopMin (admit next user)
        R-->>FS: User admitted
    end

    FS->>FS: Process order
    FS->>R: Lua: decrement stock
    FS->>U: Order confirmed
```

### Stock Buckets

Flash sale stock is distributed across `N=10` Redis buckets per SKU to reduce contention. Each bucket is an independent Redis key.

| Concept | Detail |
|---------|--------|
| Buckets | 10 per SKU (`flash:sku_101:bucket:0` — `flash:sku_101:bucket:9`) |
| Primary bucket | `hash(device_fingerprint) % 10` |
| Overflow | Sequential fallback to `(primary + 1) % N` |
| Atomic check | Lua script checks all buckets starting from primary |

```go
func (s *Service) selectBucket(sku, fingerprint string) int {
    h := fnv.New32a()
    h.Write([]byte(sku + ":" + fingerprint))
    return int(h.Sum32()) % s.bucketCount
}
```

### Waiting Room

When stock is available but demand exceeds throughput, users enter a sorted set waiting room. The score is their entry timestamp. A background goroutine admits users via `ZPopMin`.

```redis
ZADD flash:sale:123:queue <timestamp> <user_id>
ZPOPMIN flash:sale:123:queue <batch_size>
```

## Rate Limiting

Sliding window rate limiter keyed by device fingerprint. Each device gets 3 checkout attempts per 10-second window.

```go
func (s *Service) isRateLimited(ctx context.Context, fingerprint string) bool {
    key := fmt.Sprintf("rl:flash:%s", fingerprint)
    now := time.Now().Unix()

    // Remove entries outside window
    s.rdb.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now-10, 10))

    // Count entries in window
    count, _ := s.rdb.ZCard(ctx, key).Result()
    if count >= 3 {
        return true
    }

    // Add current attempt
    s.rdb.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: strconv.FormatInt(now, 10)})
    s.rdb.Expire(ctx, key, 10*time.Second)
    return false
}
```

## Attestation Tokens

Before checkout, the client requests an HMAC-SHA256 attestation token. The token embeds WebGL fingerprint, canvas fingerprint, and a CAPTCHA result. The service verifies the token on each checkout request.

```go
func (s *Service) generateToken(ctx context.Context, fingerprint string) (string, error) {
    payload := map[string]interface{}{
        "fp":  fingerprint,
        "exp": time.Now().Add(30 * time.Second).Unix(),
        "nonce": randSeq(16),
    }
    payloadBytes, _ := json.Marshal(payload)
    mac := hmac.New(sha256.New, []byte(s.secretKey))
    mac.Write(payloadBytes)
    sig := hex.EncodeToString(mac.Sum(nil))
    return base64.RawURLEncoding.EncodeToString(payloadBytes) + "." + sig, nil
}
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/checkout` | Submit a flash sale checkout request |
| `GET` | `/queue-status` | Get current position in the waiting room |
| `POST` | `/admin/sales` | Create or configure a flash sale event |

### POST /checkout

```json
// Request
{"sale_id": "sale_42", "sku": "sku_101", "qty": 1, "attestation": "eyJmcCI6Inh5eiJ9.abc123def"}

// Response 200 (admitted)
{"data": {"order_id": "ord_flash_99", "status": "confirmed"}}

// Response 202 (queued)
{"data": {"status": "queued", "position": 42, "estimated_wait_seconds": 12}}
```

### GET /queue-status?user_id=usr_42&sale_id=sale_42

```json
// Response 200
{"data": {"position": 15, "total_in_queue": 200, "estimated_wait_seconds": 8}}
```

## Key Algorithms

### Bucketed Stock Reservation (Lua)

```lua
-- KEYS[N] = bucket keys for one SKU
-- ARGV[1] = primary index
-- ARGV[2] = qty
local n = #KEYS
local start = tonumber(ARGV[1]) + 1
for i = 0, n - 1 do
    local idx = ((start + i - 1) % n) + 1
    local stock = tonumber(redis.call("GET", KEYS[idx]) or "0")
    if stock >= tonumber(ARGV[2]) then
        redis.call("DECRBY", KEYS[idx], ARGV[2])
        return { KEYS[idx], stock - tonumber(ARGV[2]) }
    end
end
return redis.error_reply("SOLD_OUT")
```

## Technical Decisions

- **10 stock buckets per SKU**: Reduces Redis key contention by an order of magnitude. With a single key, every concurrent checkout serialises on one Lua script. With 10 buckets, 10 concurrent checkouts can proceed in parallel as long as they hash to different buckets.
- **Fingerprint-based bucket assignment**: Ensures the same device always hits the same bucket, making retries predictable. Sequential fallback on overflow prevents deterministic rejection.
- **Sorted set waiting room**: Redis sorted sets provide O(log N) insertion and O(log N) removal. `ZPopMin` admits the earliest-waiting users efficiently. The room acts as a shock absorber between client demand and processing capacity.
- **Sliding window rate limiting per device**: 3 attempts per 10 seconds prevents automated bots from flooding the system while allowing legitimate retries on transient failures.
- **HMAC-SHA256 attestation**: Client-generated attestations with WebGL, canvas, and CAPTCHA challenges make automated abuse harder. The 30-second token expiry limits the replay window. The server secret key prevents forgery.
- **Background admit goroutine**: Polls the waiting room sorted set and admits users in FIFO order. The admit rate is configurable per sale based on processing capacity, preventing resource exhaustion.

## Mobile vs Web — Key Differences

The flash sale backend is designed to serve both web and mobile clients, but mobile introduces specific requirements documented here.

### Device Fingerprint

| Platform | Web | Mobile |
|----------|-----|--------|
| iOS | Browser canvas/WebGL fingerprint | `identifierForVendor` (IDFV) + **App Attest** (hardware-backed) |
| Android | Browser canvas/WebGL fingerprint | `ANDROID_ID` + **Play Integrity API** (hardware-backed) |
| Risk | Resettable via incognito/proxy | App reinstall resets installation IDs — need hardware attestation |

**Decision:** Server-side `device_fp` field accepts any client-provided fingerprint. For mobile, the client MUST send hardware attestation token (App Attest / Play Integrity) as the `attestation` field. The HMAC verification server-side never stores secrets in the client binary.

### Token Storage

Web cookies are irrelevant for native apps. Mobile clients MUST:
- **iOS**: Store token in **Keychain** (not UserDefaults)
- **Android**: Store token in **EncryptedSharedPreferences** or **Keystore**
- Send token via `Authorization: Bearer <token>` header on every request

### Polling vs Push

Polling `GET /queue-status` drains mobile battery and stops when the app is backgrounded. The service provides two alternatives:

| Method | Endpoint | Use Case |
|--------|----------|----------|
| HTTP Polling | `GET /queue-status` | Web fallback, debugging |
| SSE Stream | `GET /queue-stream` | **Mobile preferred** — server push via persistent connection |
| Future | Silent push notification | Wake up backgrounded app when position < 10 |

The SSE endpoint uses Redis pub/sub internally (`flash:queue:{productID}:{userID}`) so position updates are pushed immediately without polling overhead.

### Idempotency — Mandatory for Mobile

Mobile HTTP clients (OkHttp, URLSession, Alamofire) often auto-retry on network failure. Without idempotency, a single user tap could trigger multiple `POST /checkout` calls.

**Enforcement:** `idempotency_key` is a **required** field in `CheckoutRequest`. The backend uses Redis `SetNX` with a 30-second lock TTL and caches the result for 10 minutes. Duplicate requests within the lock window receive HTTP 409 with the cached result.

```json
{
  "product_id": "flash-indomie-2026",
  "user_id": "user-abc",
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440000",
  ...
}
```

### Network Reliability — Retry Safety

Mobile networks (4G/5G ↔ WiFi handoff, tunnels, backgrounding) are far less stable than desktop. The pipeline is designed to handle retries gracefully at every stage:

| Stage | Retry Safety |
|-------|-------------|
| Idempotency check | SetNX lock prevents concurrent duplicates |
| Rate limiter | Sliding window per deviceFP — retries counted as separate requests (by design) |
| Stock reservation | Lua atomic with TTL — reaper releases expired, never double-decrements |
| Waiting room | `ZAddNX` idempotent — joining twice returns same position |

### Deep Links for Order Navigation

After successful checkout, mobile clients should navigate via:

- **iOS**: Universal Links (`https://yourapp.com/order/ORD123`)
- **Android**: App Links (`https://yourapp.com/order/ORD123`)
- **Custom scheme fallback**: `yourapp://order/ORD123`

The `CheckoutResponse.order_id` field provides the ID for deep link construction client-side.

### Client Security Checklist

1. [ ] HMAC secret is **never** embedded in the mobile binary
2. [ ] Certificate pinning enabled to prevent MITM token interception
3. [ ] Root/jailbreak detection for high-value flash sales (defense-in-depth, not primary security)
4. [ ] Token stored in Keychain (iOS) / EncryptedSharedPreferences (Android), never plain storage
5. [ ] App Attest (iOS) / Play Integrity (Android) for hardware-backed device attestation
6. [ ] Idempotency key generated client-side as UUID v4 per checkout attempt

## Source Code

[View on GitHub](https://github.com/faisalaffan/faisalaffan-design-system/blob/dev/services/flash-sale/main.go)
