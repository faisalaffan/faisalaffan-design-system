# faisalaffan-design-system

[English](README.md)

Go monorepo berisi implementasi latihan system design. Setiap folder adalah service standalone dengan entrypoint sendiri.

## Struktur

```
├── url-shortener/       # Layanan URL shortener (:8080)
├── rate-limiter/        # Layanan rate limiter (:8081)
├── distributed-cache/   # Distributed cache (:8082)
├── chat-system/         # Sistem chat (:8083)
└── pkg/                 # Komponen bersama
    ├── consistenthash/  # Consistent hashing
    ├── bloomfilter/     # Bloom filter
    └── ratelimit/       # Algoritma rate limiting
```

## Menjalankan

```bash
# Jalankan per problem
go run ./url-shortener
go run ./rate-limiter

# Tes semua
go test ./...

# Build semua
go build ./...
```

## Dependensi

`docker-compose.yml` menyediakan Redis dan Postgres jika diperlukan:

```bash
docker compose up -d
```
