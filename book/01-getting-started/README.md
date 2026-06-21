# Getting Started

Welcome to the **faisalaffan-design-system** documentation. This monorepo implements all 12 system design problems from Alex Xu's *System Design Interview* (2nd Ed) as working Go services, grounded in the distributed systems theory from Martin Kleppmann's *Designing Data-Intensive Applications*.

**100 tests | 44 packages | 11 services | 2 shared packages**

## What This Project Covers

Each service tackles a real-world system design problem — URL shortening, rate limiting, chat, notifications, distributed IDs, key-value storage, autocomplete, news feeds, web crawling, video processing, and file storage. Every problem is implemented from scratch with its own standalone Go binary, using a consistent architectural style across the entire monorepo.

The project is organized into five batches, each grouping related services:

| # | Service | Port | Core Pattern | Batch |
|---|---------|------|-------------|-------|
| 1 | **url-shortener** | 8080 | Base62 + collision retry | 1 — Core |
| 2 | **rate-limiter** | 8081 | Sliding window + token bucket | 1 — Core |
| 3 | **chat-system** | 8082 | WebSocket rooms + broadcast | 3 — Realtime |
| 4 | **notification-system** | 8083 | Pub/sub + multi-channel | 3 — Realtime |
| 5 | **unique-id-generator** | 8084 | Snowflake 64-bit | 2 — Data |
| 6 | **key-value-store** | 8085 | Consistent hashing sharding | 2 — Data |
| 7 | **search-autocomplete** | 8086 | Trie + top-K frequency | 2 — Data |
| 8 | **news-feed** | 8087 | Fan-out on write | 4 — Scale |
| 9 | **web-crawler** | 8088 | BFS + politeness + dedup | 4 — Scale |
| 10 | **youtube** | 8089 | Metadata + search + transcode | 5 — Storage |
| 11 | **google-drive** | 8090 | Files + folders + versioning | 5 — Storage |

## How to Navigate

| Section | Contents |
|---------|----------|
| **01 — Getting Started** | Architecture overview and run instructions (this section) |
| **02 — Batch 1: Core** | Shared packages (kit, consistenthash), URL shortener, rate limiter |
| **03 — Batch 2: Data** | Unique ID generator, key-value store, search autocomplete |
| **04 — Batch 3: Realtime** | Chat system, notification system |
| **05 — Batch 4: Scale** | News feed, web crawler |
| **06 — Batch 5: Storage** | YouTube, Google Drive |
| **07 — Appendix** | Design decisions and references |

## Design Principles

- **Interface-first storage** — Every service defines a `Storage` interface. In-memory by default, swappable to Redis or Postgres without touching handlers.
- **Single `go.mod`** — One dependency graph, one `go.sum`, no workspace configuration.
- **Consistent patterns** — Gin Gonic HTTP framework, graceful shutdown, structured error handling — applied uniformly across all services.
- **Test coverage** — 100 tests across 44 packages, exercising both happy paths and edge cases.

## Quick Start

```bash
# Run any service
go run ./services/url-shortener

# Run all tests
go test ./...

# Build everything
go build ./...
```
