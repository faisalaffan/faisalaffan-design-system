# Running the Services

Each service is a standalone Go binary with its own port.

```bash
# Start any service
go run ./url-shortener          # :8080
go run ./rate-limiter           # :8081
go run ./chat-system            # :8082
go run ./notification-system    # :8083
go run ./unique-id-generator    # :8084
go run ./key-value-store        # :8085
go run ./search-autocomplete    # :8086
go run ./news-feed              # :8087
go run ./web-crawler            # :8088
go run ./youtube                # :8089
go run ./google-drive           # :8090
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
