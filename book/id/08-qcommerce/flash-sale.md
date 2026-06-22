# Layanan Flash Sale

300% oversell. 12.000 pesanan dibatalkan. Semua karena race condition dengan jendela 50 mikrodetik.

Skrip Lua yang bisa mencegahnya? 20 baris. Satu panggilan Redis atomik. Nggak perlu Paxos. Nggak perlu Raft. Nggak perlu distributed lock.

Setiap platform flash sale perangnya sama. Lima lapis pengaman. Sekali checkout. Nggak ada celah.

Port **8103** | Paket `flash-sale/` | 5 file: `types.go`, `store.go`, `service.go`, `handler.go`, `main.go`

## Arsitektur

```mermaid
%%{init: {"theme": "base", "themeVariables": {"background": "#ffffff"}}}%%
sequenceDiagram
    participant U as User
    participant FS as Flash Sale Service
    participant R as Redis

    Note over U,FS: Penerbitan Token
    U->>FS: GET /flash-sale/token?device_fp=X
    FS->>FS: HMAC-SHA256(device_fp, server_secret)
    FS-->>U: Token atestasi

    Note over FS,R: Pipeline Checkout (5 safety nets)
    U->>FS: POST /flash-sale/checkout
    FS->>R: 1. SetNX idempotensi
    FS->>FS: 2. Verifikasi HMAC atestasi
    FS->>R: 3. Rate limit sliding window
    FS->>R: 4. Pengurangan stok atomik Lua
    FS->>R: 5. ZAdd ruang tunggu sorted set
    FS-->>U: Posisi antrean / konfirmasi
```

## Safety Nets

### 1. Pengurangan Stok Atomik Lua

`GET -> cek -> DECRBY` keliatan aman di kertas. Kenyataannya? Bencana.

Tiga request konkuren cek stok=100. Semua lihat stok=100. Semua lolos. Sekarang stok -200. Lo berutang 200 unit.

Ini bukan teori. 2024, platform flash sale di Asia Tenggara. Stok Indomie 100 unit. Oversell 300%. 12.000 pesanan dibatalkan. CS kewalahan berminggu-minggu.

Solusinya? Satu skrip Lua. `GET` + `DECRBY` dalam satu panggilan Redis atomik. Nggak ada celah antara cek dan kurang.

```lua
-- KEYS[1] = stock key, ARGV[1] = qty
local stock = tonumber(redis.call("GET", KEYS[1]) or "0")
if stock >= tonumber(ARGV[1]) then
    redis.call("DECRBY", KEYS[1], ARGV[1])
    return {1, stock - tonumber(ARGV[1])}
end
return {0, "sold_out"}
```

### 2. Rate Limit Sliding Window per Perangkat

Rate limit berbasis IP? Sia-sia buat bot farm.

Seribu proxy residensial bergantian — tiap request datang dari IP beda. Limit-nya nggak pernah kepicu.

Limit ini pakai `device_fp`, bukan IP. Bot dengan 1000 proxy tetap cuma dapet 20 percobaan per jendela 10 detik. Limit-nya nempel di perangkat, bukan jaringan.

```go
func (s *Store) IsRateLimited(deviceFP string) bool {
    key := "rl:flash:" + deviceFP
    now := time.Now().Unix()
    s.rdb.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(now-10, 10))
    count, _ := s.rdb.ZCard(ctx, key).Result()
    if count >= 20 {
        return true
    }
    s.rdb.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
    s.rdb.Expire(ctx, key, 10*time.Second)
    return false
}
```

### 3. Atestasi HMAC (Verifikasi Lokal)

Token dari server. Server tanda tangan sidik jari perangkat pake rahasia yang nggak pernah keluar dari server. Toleransi beda jam +/-30 detik.

Dekompilasi APK. Reverse engineer IPA. Rahasianya nggak ada — karena emang nggak pernah dikirim ke client. Setiap request diverifikasi di server. Client nggak bisa palsuin token karena nggak punya kuncinya.

```go
func generateToken(deviceFP string, secret []byte) (string, error) {
    window := fmt.Sprintf("%s:%d", deviceFP, time.Now().Unix()/30)
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(window))
    sig := hex.EncodeToString(mac.Sum(nil))
    return base64.RawURLEncoding.EncodeToString([]byte(window)) + "." + sig, nil
}
```

### 4. Ruang Tunggu Sorted Set

Stok habis dalam milidetik. Yang telat harus antre. Tapi "antre" berarti urutan — dan urutan butuh persistensi.

Redis sorted set. Skor = timestamp masuk. Antrean selamat dari restart — data di disk, bukan di RAM. `ZRank` ngasih posisi dalam O(log N). Goroutine latar belakang ngadmit user secara FIFO ketat lewat `ZPopMin`.

```redis
ZADD flash:sale:{sale_id}:queue <timestamp> <user_id>
ZRANK flash:sale:{sale_id}:queue <user_id>
ZPOPMIN flash:sale:{sale_id}:queue <batch_size>
```

### 5. Pengaman Idempotensi

SDK mobile auto-retry pas jaringan gagal. OkHttp. URLSession. Itu fitur — sampe dua POST identik bikin dua pesanan.

Redis `SetNX` ngasih lock atomik dengan TTL 30 detik. Hasil checkout di-cache 10 menit. `Idempotency_key` sama? HTTP 409 lengkap sama response yang di-cache. Nggak ada pesanan kedua. Nggak ada tagihan dobel.

```go
ok, _ := s.rdb.SetNX(ctx, "idemp:"+key, result, 30*time.Second).Result()
```

## API Endpoints

| Method | Path | Deskripsi |
|--------|------|-----------|
| POST | `/flash-sale/checkout` | Pipeline lengkap: idempotensi -> atestasi -> rate limit -> stok -> antrean |
| POST | `/flash-sale/release` | Kompensasi checkout gagal, kembalikan stok secara atomik |
| GET | `/flash-sale/queue-status` | Polling posisi antrean via ZRank |
| GET | `/flash-sale/token` | Terbitkan token atestasi HMAC-SHA256 |

### POST /flash-sale/checkout

```json
// Request
{
  "product_id": "flash-indomie-2026",
  "user_id": "user-abc",
  "qty": 1,
  "attestation": "ZXlKcGNDSTZJbmgx...",
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
}

// 200 — dikonfirmasi
{"data": {"order_id": "ord_flash_99", "status": "confirmed"}}

// 202 — dalam antrean
{"data": {"status": "queued", "position": 42, "estimated_wait_seconds": 12}}

// 429 — kena rate limit
{"error": "too_many_requests", "retry_after_ms": 800}

// 409 — duplikat (idempotency_key sama)
{"error": "duplicate_request", "data": {"order_id": "ord_flash_99", "status": "confirmed"}}
```

### GET /flash-sale/queue-status?product_id=flash-indomie-2026&user_id=user-abc

```json
{"data": {"position": 15, "total_in_queue": 200, "estimated_wait_seconds": 8}}
```

## Uji Skenario — Validasi Desain

| # | Skenario | Masalah yang Diselesaikan | Tanpa Desain Ini |
|---|----------|---------------------------|------------------|
| 1 | Happy path checkout | Pipeline penuh validasi setiap langkah atomik | Stok hilang saat gagal di tengah, tanpa jalur pemulihan |
| 2 | Double-tap karena panik | SetNX idempotensi cegah pesanan duplikat | Auto-retry bikin 2 pesanan, tagihan dobel |
| 3 | 200 user rebut 100 stok | Lua atomik cek+kurang dalam satu operasi | Race condition -> oversell 300% (12K pesanan dibatalkan) |
| 4 | Bot token palsu + 30 spam | Atestasi HMAC (401) + rate limit burst=20 (429) | Bot boros stok dalam <1s via 1000 proxy |
| 5 | User ke-101 masuk ruang tunggu | Sorted set persisten, ZRank untuk posisi real-time | Antrean in-memory hilang saat deploy |
| 6 | Timeout payment gateway 15s | POST /release kembalikan stok atomik, idempoten | Stok terkunci selamanya -> ghost stock permanent |
| 7 | Token 31s lewat expiry | +/-30s toleransi jam, verifikasi HMAC server-side | User dengan selisih jam 5s selalu ditolak |
| 8 | Endpoint penerbitan token | Secret HMAC cuma di server, nggak pernah di binary | Secret di APK/IPA -> didekompilasi -> bot bikin token valid |
| 9 | 25 request dari perangkat sama | Sliding window per device_fp, burst=20 | Limit IP: 1000 proxy -> 5000 request, bypass total |
| 10 | Handoff mobile (4G->WiFi), OkHttp retry | Idempotency_key sama -> 409 + hasil cache | 2 POST identik -> 2 pesanan, 2 tagihan |
| 11 | Admit terlambat saat antrean mengalir | ZAddNX cegah re-add, ZPopMin admit strictly FIFO | User gabung ulang dengan score manipulatif buat loncat antrean |

## Keputusan Teknis

| Keputusan | Alasan |
|-----------|--------|
| Pengurangan stok atomik Lua | Operasi Redis tunggal eliminasi race condition cek-kurang. Cegah oversell tanpa distributed lock atau konsensus. |
| Rate limit per sidik jari perangkat | Ikat limit ke perangkat, bukan IP. Bot farm 1000 proxy nggak bisa bypass karena limit ngikut identitas perangkat. |
| Atestasi HMAC (verifikasi lokal) | Secret cuma di server — nggak bisa bocor dari binary klien. +/-30s toleransi jam tanpa memperlebar window replay. |
| Ruang tunggu Redis sorted set | Antrean persisten selamat dari restart. ZRank ngasih posisi O(log N) real-time. Skalabel ke jutaan pengguna. |
| Pengaman idempotensi SetNX | Lock+cache atomik cegah duplikat dari auto-retry mobile. Lock 30s cukup buat checkout; cache hasil cegah pemrosesan ulang. |

## Source Code

[View on GitHub](https://github.com/faisalaffan/faisalaffan-design-system/blob/dev/services/flash-sale/main.go)
