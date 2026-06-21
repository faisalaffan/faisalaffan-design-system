# Running the Services

Each service is a standalone Go binary with its own port.

```bash
# Start any service
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
```

## Verify

```bash
go test ./...    # 100 tests across 44 packages
go build ./...   # Build all
go vet ./...     # Static analysis
```

## Dependencies

```bash
docker compose up -d   # Redis + Postgres (for future persistence layers)
```
