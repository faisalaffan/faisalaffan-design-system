# faisalaffan-design-system

[Bahasa Indonesia](README.id.md)

## 01_LOGO

<p align="center">
  <img src="assets/logo/logo.svg" alt="faisalaffan-design-system logo" width="400"/>
</p>

## 02_BANNER

<p align="center">
  <img src="assets/banner/banner.svg" alt="banner" width="800"/>
</p>

---

Go monorepo for system design exercise implementations. Each folder is a standalone service with its own entrypoint.

## Architecture

```mermaid
flowchart TB
    subgraph "Batch 1"
        US["url-shortener<br/>:8080"]
        RL["rate-limiter<br/>:8081"]
    end

    subgraph "Batch 2"
        ID["unique-id-generator<br/>:8084"]
        KV["key-value-store<br/>:8085"]
        AC["search-autocomplete<br/>:8086"]
    end

    subgraph "Shared (pkg/)"
        KIT["kit<br/>Gin factory"]
        CH["consistenthash<br/>Hash ring"]
    end

    US --> KIT
    RL --> KIT
    ID --> KIT
    KV --> KIT
    KV --> CH
    AC --> KIT

    subgraph "Coming Soon (Batch 3-5)"
        DC["distributed-cache"]
        CS["chat-system"]
        NF["news-feed"]
        WC["web-crawler"]
    end

    style KIT fill:#009688,color:#fff
    style CH fill:#ff9800,color:#000
```

```mermaid
sequenceDiagram
    participant C as Client
    participant ID as ID Generator
    participant US as URL Shortener
    participant KV as Key-Value Store
    participant AC as Autocomplete

    C->>ID: GET /id
    ID-->>C: {id: 123456789}

    C->>US: POST /shorten {url}
    US-->>C: 201 {short_url, code}

    C->>KV: PUT /mykey {value}
    KV-->>C: 200 {stored: true}
    C->>KV: GET /mykey
    KV-->>C: 200 {value}

    C->>AC: POST /train/bulk {terms}
    AC-->>C: 200 {trained: N}
    C->>AC: GET /autocomplete?q=pref
    AC-->>C: {results: [...]}
```

## Structure

```
├── url-shortener/          # URL shortener (:8080)
│   ├── handler/            # Gin HTTP handlers
│   ├── service/            # Business logic
│   ├── storage/            # Storage interface + in-memory
│   └── shortcode/          # Base62 random code generator
├── rate-limiter/           # Rate limiter (:8081)
│   ├── algorithm/          # Token bucket, sliding window, fixed window
│   └── storage/            # Counter store interface + in-memory
├── unique-id-generator/    # Snowflake ID generator (:8084)
│   ├── snowflake/          # 64-bit ID: timestamp + worker + sequence
│   └── handler/            # GET /id
├── key-value-store/        # Distributed KV store (:8085)
│   ├── storage/            # Store interface + in-memory
│   ├── shard/              # Consistent hashing shard manager
│   └── handler/            # GET/PUT/DELETE /:key
├── search-autocomplete/    # Autocomplete service (:8086)
│   ├── trie/               # Concurrent-safe prefix tree
│   └── handler/            # GET /autocomplete, POST /train
├── distributed-cache/      # — soon
├── chat-system/            # — soon
└── pkg/                    # Shared components
    ├── kit/                # Gin factory, config, response helpers, middleware
    └── consistenthash/     # Consistent hashing with virtual nodes
```

## Running

```bash
go run ./url-shortener          # :8080
go run ./rate-limiter           # :8081
go run ./unique-id-generator    # :8084
go run ./key-value-store        # :8085
go run ./search-autocomplete    # :8086

go test ./...                   # 48 tests across 25 packages
go build ./...                  # Build all
go vet ./...                    # Vet all
```

## Dependencies

```bash
docker compose up -d       # Redis + Postgres (for future problems)
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
| **Trie-based autocomplete** | O(k) prefix search where k = input length. Top-K by frequency with lexicographic tiebreak. Concurrent-safe reads. |
| **Graceful shutdown** | Every service handles SIGINT/SIGTERM with 5s timeout. Production pattern in exercise code. |
