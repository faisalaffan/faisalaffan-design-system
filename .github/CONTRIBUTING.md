# Contributing

## Project Structure

```
├── <service-name>/     # Each service is standalone
│   ├── main.go         # Entrypoint
│   ├── handler/        # HTTP layer (Gin)
│   ├── service/        # Business logic
│   └── storage/        # Data layer (interface + impl)
├── pkg/                # Shared packages
└── .github/            # CI + templates
```

## Conventions

- **Go 1.26+**, Gin Gonic for HTTP
- **Interface-first**: storage, algorithm, and channel interfaces before concrete implementations
- **In-memory default**: all storage defaults to in-memory; swap to Redis/Postgres by implementing the interface
- **Tests**: `_test.go` alongside code. Run `go test ./...` from root.
- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/) — `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`

## Adding a New Service

1. Create folder: `<service-name>/`
2. Define storage interface in `<service-name>/storage/storage.go`
3. Implement business logic in `<service-name>/service/`
4. Wire Gin handlers in `<service-name>/handler/`
5. Bootstrap server with `pkg/kit` in `<service-name>/main.go`
6. Add tests. Run `go test ./<service-name>/... -v`
7. Update README.md with Mermaid diagram

## PR Checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (100%)
- [ ] `go vet ./...` clean
- [ ] README updated with architecture diagram
