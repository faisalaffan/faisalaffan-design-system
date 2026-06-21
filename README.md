# faisalaffan-design-system

<p align="center">
  <a href="README.id.md">🇮🇩 Bahasa Indonesia</a>
</p>

<p align="center">
  <img src="assets/01_BANNER.png" alt="banner" width="800"/>
</p>

<p align="center">
  <img src="assets/01_LOGO.png" alt="faisalaffan-design-system logo" width="200"/>
</p>

---

Go monorepo implementing all 12 system design problems from Alex Xu's "System Design Interview" (2nd Ed), backed by theory from Kleppmann's "Designing Data-Intensive Applications".

**203 tests | 92 packages | 22 services | 2 shared packages**

## Services

| # | Service | Port | Core Pattern | Batch |
|---|---------|------|-------------|-------|
| 1 | **url-shortener** | 8080 | Base62 + collision retry | 1 |
| 2 | **rate-limiter** | 8081 | Sliding window + token bucket | 1 |
| 3 | **chat-system** | 8082 | WebSocket rooms + broadcast | 3 |
| 4 | **notification-system** | 8083 | Pub/sub + multi-channel | 3 |
| 5 | **unique-id-generator** | 8084 | Snowflake 64-bit | 2 |
| 6 | **key-value-store** | 8085 | Consistent hashing sharding | 2 |
| 7 | **search-autocomplete** | 8086 | Trie + top-K frequency | 2 |
| 8 | **news-feed** | 8087 | Fan-out on write | 4 |
| 9 | **web-crawler** | 8088 | BFS + politeness + dedup | 4 |
| 10 | **youtube** | 8089 | Metadata + search + transcode | 5 |
| 11 | **google-drive** | 8090 | Files + folders + versioning | 5 |

### Q-Commerce

| # | Service | Port | Core Pattern |
|---|---------|------|-------------|
| 12 | **inventory-service** | 8100 | Redis Lua atomic reservation, dual-store, TTL reaper |
| 13 | **geo-service** | 8101 | H3 hexagon indexing, haversine tie-breaking |
| 14 | **flash-sale** | 8102 | Stock buckets, waiting room, HMAC attestation |
| 15 | **checkout-service** | 8103 | Saga orchestration, outbox, idempotency |
| 16 | **promo-engine** | 8104 | AST rule tree, Redis Lua counters, fraud detection |
| 17 | **dispatch-service** | 8105 | Batch collector, greedy matcher, driver state machine |
| 18 | **eta-service** | 8106 | Concurrent estimation, Kalman, sticky ETA cache |
| 19 | **search-service** | 8107 | Multi-match ranking, trie autocomplete, composite score |
| 20 | **tracking-service** | 8108 | WebSocket ingestion, Kalman filter, SSE push |
| 21 | **pricing-service** | 8109 | Surge detection, price lock, elasticity tracker |
| 22 | **forecasting-service** | 8110 | Holt-Winters engine, Bayesian cold-start, replenishment |


## Running

```bash
go run ./services/url-shortener          # :8080
go run ./services/rate-limiter           # :8081
go run ./services/chat-system            # :8082
go run ./services/notification-system    # :8083
go run ./services/unique-id-generator    # :8084
go run ./services/key-value-store        # :8085
go run ./services/search-autocomplete    # :8086
go run ./services/news-feed              # :8087
go run ./services/web-crawler            # :8088
go run ./services/youtube                # :8089
go run ./services/google-drive           # :8090
go run ./services/inventory-service      # :8100
go run ./services/geo-service            # :8101
go run ./services/flash-sale             # :8102
go run ./services/checkout-service       # :8103
go run ./services/promo-engine           # :8104
go run ./services/dispatch-service       # :8105
go run ./services/eta-service            # :8106
go run ./services/search-service         # :8107
go run ./services/tracking-service       # :8108
go run ./services/pricing-service        # :8109
go run ./services/forecasting-service    # :8110

go test ./...                   # 203 tests across 92 packages
go build ./...                  # Build all
go vet ./...                    # Vet all
```

## Dependencies

```bash
docker compose up -d       # Redis + Postgres (for future persistence)
```

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Gin Gonic** | Fastest Go HTTP framework. Middleware ecosystem. Productive without sacrificing performance. |
| **Interface-first storage** | Every service defines a `Storage` interface. In-memory default, swappable to Redis/Postgres with zero handler changes. |
| **Single `go.mod`** | Solo portfolio repo. Shared dependency versions. Multi-module overkill for this scale. |
| **Base62 random shortcode** | 7 chars from [a-zA-Z0-9] = ~3.5 trillion combinations. Random + retry simpler than URL hashing. |
| **Sliding window rate limiting** | More precise than fixed window. Better memory profile than token bucket for high-cardinality keys. |
| **Virtual nodes (150 replicas)** | Consistent hashing with 150 virtual nodes. `crc32` for speed — sufficient uniformity. |
| **Snowflake 64-bit IDs** | Timestamp(41) + Worker(10) + Sequence(12) = 4096 IDs/ms/worker, ~69 year lifetime. No coordination needed. |
| **Trie-based autocomplete** | O(k) prefix search. Top-K by frequency with lexicographic tiebreak. Concurrent-safe reads. |
| **Fan-out on write** | Push new posts to all follower timelines at write time. Trade write amplification for instant timeline reads. |
| **BFS crawler with politeness** | Channel-based URL frontier. Per-domain delay. HTML link extraction via `golang.org/x/net/html`. |
| **WebSocket rooms** | Goroutine-based room event loop (join/leave/broadcast channels). Message history ring buffer (100 msg cap). |
| **Multi-channel notification** | Sender interface: in-app (persisted), email, push (simulated). Pub/sub decouples publishers from delivery. |
| **Simulated transcoding** | Goroutine-based async video processing. State machine: uploading → processing → ready. |
| **File versioning** | Immutable version history per file. Each update appends new version. Old content retained for rollback. |
| **Graceful shutdown** | Every service handles SIGINT/SIGTERM with 5s timeout. Production pattern in exercise code. |
