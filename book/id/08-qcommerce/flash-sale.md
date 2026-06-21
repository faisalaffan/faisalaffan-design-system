# Layanan Flash Sale

Layanan flash sale dengan bucket stok Redis Lua (N=10), ruang tunggu berbasis sorted set, rate limit sliding window per sidik jari perangkat, dan token atestasi HMAC-SHA256 untuk mencegah pembelian bot.

Port **8093** | Paket `flash-sale/`

---

## Arsitektur

```mermaid
%%{init: {"theme": "base", "themeVariables": {"background": "#ffffff"}}}%%
sequenceDiagram
    participant C as Client
    participant GW as API Gateway
    participant FS as Flash Sale Service
    participant RL as Rate Limiter
    participant R as Redis
    participant HMAC as HMAC Attestation

    Note over C,R: Persiapan: Client meminta token
    C->>GW: GET /flash-sale/token?device_fingerprint=ABC
    GW->>FS: minta token
    FS->>HMAC: HMAC-SHA256(device_fp + secret)
    HMAC-->>FS: token atestasi
    FS-->>GW: {"token": "abc123..."}
    GW-->>C: token disimpan di cookie/header

    Note over C,R: Checkout Flash Sale
    C->>GW: POST /checkout {"sku":"SKU001","qty":1}
    Note over GW: Validasi HMAC token
    GW->>RL: sliding window rate limit
    RL-->>GW: allowed / blocked

    GW->>FS: proses checkout
    FS->>R: EVALSHA (skrip Lua bucket stok)
    R-->>FS: reserved / sold out

    FS->>R: ZADD waiting_room:sale_id timestamp
    R-->>FS: posisi antrean

    FS-->>GW: {"status":"processing","queue":42}
    GW-->>C: polling queue-status

    C->>GW: GET /queue-status?order_id=X
    GW->>FS: cek status
    FS->>R: ZSCORE / ZRANK
    R-->>FS: posisi / selesai
    FS-->>GW: {"status":"completed","order_id":"ORD123"}
    GW-->>C: redirect ke order detail
```

## Komponen

### Bucket Stok Redis Lua

Stok flash sale dipartisi ke N bucket Redis untuk mengurangi kontensi. Skrip Lua menangani reservasi atomik.

```lua
-- flash_sale_reserve.lua
local bucket_count = tonumber(ARGV[1])   -- total bucket (N)
local qty          = tonumber(ARGV[2])   -- quantity diminta
local sale_id      = KEYS[1]             -- "flash_sale:{sale_id}"
local bucket_key   = KEYS[2]             -- "flash_sale:{sale_id}:bucket:{n}"

-- coba bucket demi bucket
for i = 0, bucket_count - 1 do
    local bk = bucket_key .. i
    local stock = redis.call("GET", bk)
    if stock and tonumber(stock) >= qty then
        redis.call("DECRBY", bk, qty)
        return {1, "reserved", i}
    end
end
return {0, "sold out", -1}
```

### Ruang Tunggu (Waiting Room) dengan Sorted Set

Setiap pengguna yang berhasil reservasi masuk ke antrean pemrosesan via Redis sorted set dengan timestamp sebagai skor.

```go
func enqueue(ctx context.Context, saleID, userID string) (int64, error) {
    now := time.Now().UnixMicro()
    key := fmt.Sprintf("waiting_room:%s", saleID)
    rank, err := redis.ZAdd(ctx, key, &redis.Z{
        Score:  float64(now),
        Member: userID,
    }).Result()
    if err != nil {
        return 0, err
    }
    // rank = 1-based position in queue
    position, _ := redis.ZRank(ctx, key, userID).Result()
    return position + 1, nil
}
```

### Rate Limit Sliding Window per Sidik Jari Perangkat

Rate limiting berbasis sliding window log dengan granularitas per `device_fingerprint`.

```go
func rateLimitCheck(ctx context.Context, fingerprint string) bool {
    now := time.Now().UnixMilli()
    window := int64(1000) // 1 detik
    limit := 3           // maks 3 request per detik

    key := fmt.Sprintf("rl:flash:%s", fingerprint)
    // hapus entri di luar window
    redis.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now-window, 10))
    count, _ := redis.ZCard(ctx, key).Result()
    if count >= limit {
        return false
    }
    redis.ZAdd(ctx, key, &redis.Z{Score: float64(now), Member: strconv.FormatInt(now, 10)})
    redis.Expire(ctx, key, 2*time.Second)
    return true
}
```

### Atestasi HMAC-SHA256

Token atestasi mencegah bot dengan memvalidasi bahwa request berasal dari klien yang sudah melewati tantangan sebelumnya.

```go
func generateAttestationToken(deviceFP string, secret []byte) string {
    payload := fmt.Sprintf("%s:%d", deviceFP, time.Now().Unix()/30) // 30s window
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(payload))
    signature := hex.EncodeToString(mac.Sum(nil))
    return fmt.Sprintf("%s.%s", deviceFP, signature)
}

func validateAttestationToken(token string, secret []byte) bool {
    parts := strings.SplitN(token, ".", 2)
    if len(parts) != 2 {
        return false
    }
    expected := generateAttestationToken(parts[0], secret)
    return hmac.Equal([]byte(expected), []byte(token))
}
```

## API Endpoints

| Method | Path | Deskripsi |
|--------|------|-------------|
| `GET` | `/flash-sale/token` | Mendapatkan token atestasi HMAC |
| `POST` | `/checkout` | Melakukan checkout flash sale |
| `GET` | `/queue-status?order_id=` | Mengecek status antrean |

### POST /checkout

```json
// Request
{
  "sale_id": "SALE001",
  "sku": "SKU001",
  "qty": 1,
  "device_fingerprint": "abc123",
  "attestation_token": "abc123.def..."
}

// Response 200 (diproses)
{
  "status": "processing",
  "order_id": "ORD123",
  "queue_position": 42,
  "estimated_seconds": 5
}

// Response 429 (rate limited)
{"error": "too many requests", "retry_after_ms": 800}

// Response 410 (stok habis)
{"error": "sold out"}

// Response 403 (token tidak valid)
{"error": "invalid attestation token"}
```

### GET /queue-status?order_id=

```json
// Response 200
{
  "status": "processing",
  "queue_position": 15,
  "total_ahead": 14
}

// Response 200 (selesai)
{
  "status": "completed",
  "order_id": "ORD123",
  "redirect_url": "/orders/ORD123"
}
```

## Algoritma Kunci

| Algoritma | Penggunaan |
|-----------|------------|
| Lua bucket scanning | Iterasi N bucket, DECRBY pada bucket pertama yang cukup |
| Sorted set waiting room | ZADD untuk antrean, ZRANK/ZSCORE untuk polling |
| Sliding window log | Redis ZADD + ZREMRANGEBYSCORE per device_fingerprint |
| HMAC-SHA256 attestation | Token dengan window 30 detik, validasi server-side |
| Polling with backoff | Klien polling GET /queue-status dengan interval eksponensial |

## Keputusan Teknis

- **N=10 bucket stok**: Jumlah bucket yang cukup untuk mendistribusikan kontensi Redis di flash sale dengan lalu lintas tinggi (ribuan request per detik). Bucket lebih banyak meningkatkan throughput tetapi menambah overhead manajemen. Nilai N bisa dikonfigurasi per flash sale.
- **Waiting room sorted set menggantikan queue in-memory**: Redis sorted set dengan timestamp sebagai skor memberikan antrean terdistribusi yang persisten. Jika instance service mati, antrean tidak hilang. ZRANK memberi posisi pengguna secara real-time.
- **Sliding window, bukan fixed window**: Flash sale memiliki pola ledakan permintaan yang ekstrem. Fixed window bisa menyebabkan 3x lipat traffic di perbatasan window. Sliding window memberikan batas yang halus dan akurat.
- **Token HMAC dengan window 30 detik**: Token atestasi memiliki masa berlaku pendek untuk membatasi jendela serangan replay. Window 30 detik memberi toleransi perbedaan jam klien-server tanpa membuka celah serangan yang besar.
- **Polling klien untuk status antrean, bukan WebSocket**: Flash sale bersifat fire-and-forget; klien tidak perlu koneksi persisten. Polling GET dengan backoff eksponensial (1s, 2s, 4s, 8s, maks 10s) lebih sederhana dan sama efektifnya.
