# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go monorepo for system design exercise implementations. Single module, multiple entrypoints — each folder is a standalone service/problem.

## Build & Run

```bash
# Run any problem directly
go run ./services/url-shortener
go run ./services/rate-limiter

# Build all
go build ./...

# Run all tests
go test ./...

# Run tests for specific folder
go test ./services/url-shortener/...
go test ./services/rate-limiter/handler/...

# Vet and lint
go vet ./...
golangci-lint run ./...  # if golangci-lint installed
```

## Architecture

```
go.mod                        # single module (not multi-module)
go.sum
services/                     # 11 system design services
  url-shortener/
  rate-limiter/
  chat-system/
  notification-system/
  unique-id-generator/
  key-value-store/
  search-autocomplete/
  news-feed/
  web-crawler/
  youtube/
  google-drive/
pkg/                          # shared code across problems
  kit/                        # Gin factory, config, response helpers, middleware
  consistenthash/             # Hash ring with virtual nodes
```

**Module decision**: Single `go.mod` at root. No `go work` workspace. All problems share same Go version and dependency set. Trade-off: `go.sum` accumulates all deps but acceptable for solo/portfolio repo.

**Import convention**:
```go
import "github.com/faisalaffan/faisalaffan-design-system/pkg/consistenthash"
```

**Shared code (`pkg/`)**: Reusable components (consistent hashing, bloom filter, LRU cache, rate limiter algorithm) live here. Import with full module path. Be mindful that changes to `pkg/` affect all problems — test after modifying.

**No package name collisions**: Each problem folder is a self-contained `package main`. Different folders with same-named sub-packages don't collide since you run one problem at a time.

## Conventions

- Go version set once in root `go.mod` — if one problem needs newer Go features (generics), ensure backward compat or bump module version
- Tests alongside code in `_test.go` files within each package
- README per problem folder explaining its design and how to run
