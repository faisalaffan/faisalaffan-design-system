# Architecture

## High-Level Design

```mermaid
flowchart TB
    subgraph "Batch 1 — Core"
        US["url-shortener :8080"]
        RL["rate-limiter :8081"]
    end
    subgraph "Batch 2 — Data"
        ID["unique-id-generator :8084"]
        KV["key-value-store :8085"]
        AC["search-autocomplete :8086"]
    end
    subgraph "Batch 3 — Realtime"
        CS["chat-system :8082"]
        NS["notification-system :8083"]
    end
    subgraph "Batch 4 — Scale"
        NF["news-feed :8087"]
        WC["web-crawler :8088"]
    end
    subgraph "Batch 5 — Storage"
        YT["youtube :8089"]
        GD["google-drive :8090"]
    end
    subgraph "Shared"
        KIT["pkg/kit"]
        CH["pkg/consistenthash"]
    end
    US --> KIT
    RL --> KIT
    ID --> KIT
    KV --> KIT
    KV --> CH
    AC --> KIT
    CS --> KIT
    NS --> KIT
    NF --> KIT
    WC --> KIT
    YT --> KIT
    GD --> KIT
```

## Shared Packages

### pkg/kit
Gin factory, config loader, JSON response helpers, AppError types, and middleware (recovery, logging, rate-limit bridge).

### pkg/consistenthash
Hash ring with 150 virtual nodes per physical node. `crc32` hashing. Used by key-value-store for shard-to-node mapping.

## Design Philosophy

- **Interface-first**: every service defines a `Storage` interface. In-memory default, swappable to Redis/Postgres.
- **Single module**: one `go.mod` at root. All services share dependency versions.
- **Graceful shutdown**: every service handles SIGINT/SIGTERM with 5s timeout.
- **Gin Gonic**: consistent HTTP framework across all services.
