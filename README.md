# faisalaffan-design-system

[Bahasa Indonesia](README.id.md)

Go monorepo for system design exercise implementations. Each folder is a standalone service with its own entrypoint.

## Structure

```
├── url-shortener/       # URL shortener service (:8080)
├── rate-limiter/        # Rate limiter service (:8081)
├── distributed-cache/   # Distributed cache (:8082)
├── chat-system/         # Chat system (:8083)
└── pkg/                 # Shared components
    ├── consistenthash/  # Consistent hashing
    ├── bloomfilter/     # Bloom filter
    └── ratelimit/       # Rate limiting algorithms
```

## Running

```bash
# Run per problem
go run ./url-shortener
go run ./rate-limiter

# Test all
go test ./...

# Build all
go build ./...
```

## Dependencies

`docker-compose.yml` provides Redis and Postgres when needed:

```bash
docker compose up -d
```
