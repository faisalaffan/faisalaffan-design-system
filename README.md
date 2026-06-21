# faisalaffan-design-system

[Bahasa Indonesia](README.id.md)

Go monorepo for system design exercise implementations. Each folder is a standalone service with its own entrypoint.

## Architecture

```mermaid
flowchart TB
    subgraph "Services"
        US["url-shortener<br/>:8080<br/>Gin + base62"]
        RL["rate-limiter<br/>:8081<br/>Gin + sliding window"]
    end

    subgraph "Shared (pkg/)"
        KIT["kit<br/>Gin factory + middleware"]
        CH["consistenthash<br/>Hash ring + virtual nodes"]
    end

    US --> KIT
    RL --> KIT
    RL --> CH

    subgraph "Coming Soon"
        DC["distributed-cache<br/>:8082"]
        CS["chat-system<br/>:8083"]
    end

    style KIT fill:#009688,color:#fff
    style CH fill:#ff9800,color:#000
```

```mermaid
sequenceDiagram
    participant C as Client
    participant US as URL Shortener
    participant RL as Rate Limiter
    participant CH as Consistent Hash

    C->>RL: Rate-limited request
    RL-->>C: Allow / Deny

    C->>US: POST /shorten
    US->>US: Generate base62 code
    US-->>C: 201 {short_url, code}

    C->>US: GET /:code
    US-->>C: 302 Redirect

    C->>CH: Lookup node for key
    CH-->>C: Target node
```

## Structure

```
├── url-shortener/       # URL shortener service (:8080)
│   ├── handler/         # Gin HTTP handlers
│   ├── service/         # Business logic
│   ├── storage/         # Storage interface + in-memory impl
│   └── shortcode/       # Base62 random code generator
├── rate-limiter/        # Rate limiter service (:8081)
│   ├── algorithm/       # Token bucket, sliding window, fixed window
│   └── storage/         # Counter store interface + in-memory impl
├── distributed-cache/   # Distributed cache (:8082) — soon
├── chat-system/         # Chat system (:8083) — soon
└── pkg/                 # Shared components
    ├── kit/             # Gin factory, config, response helpers, middleware
    └── consistenthash/  # Consistent hashing with virtual nodes
```

## Running

```bash
go run ./url-shortener     # :8080
go run ./rate-limiter      # :8081

go test ./...              # Run all tests
go build ./...             # Build all
go vet ./...               # Vet all
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
| **Base62 random shortcode** | 7 chars from [a-zA-Z0-9] = ~3.5 trillion combinations. Random + retry simpler than URL hashing, avoids same-URL race conditions. |
| **Sliding window rate limiting** | Default algorithm. More precise than fixed window (no edge-of-window burst). Better memory profile than token bucket for high-cardinality keys. |
| **Virtual nodes (150 replicas)** | Consistent hashing with 150 virtual nodes per physical node. `crc32` for speed — sufficient uniformity for non-crypto use case. |
| **Graceful shutdown** | Each service handles SIGINT/SIGTERM with 5s timeout. Production pattern even in exercise code. |
