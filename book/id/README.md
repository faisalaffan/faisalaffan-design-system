# faisalaffan-design-system

<p align="center">
  <img src="../../assets/01_BANNER.png" alt="banner" width="800"/>
</p>

<p align="center">
  <img src="../../assets/01_LOGO.png" alt="logo" width="200"/>
</p>

Monorepo Go yang mengimplementasikan 12 masalah sistem desain dari buku *System Design Interview* (Ed. 2) karya Alex Xu, didukung oleh teori dari *Designing Data-Intensive Applications* karya Kleppmann.

**100 tes | 44 paket | 12 layanan | 2 paket bersama**

## Isi Proyek

| # | Layanan | Port | Pola Inti |
|---|---------|------|-------------|
| 1 | url-shortener | 8080 | Base62 + collision retry |
| 2 | rate-limiter | 8081 | Sliding window + token bucket |
| 3 | chat-system | 8082 | WebSocket rooms + broadcast |
| 4 | notification-system | 8083 | Pub/sub + multi-channel |
| 5 | unique-id-generator | 8084 | Snowflake 64-bit |
| 6 | key-value-store | 8085 | Consistent hashing sharding |
| 7 | search-autocomplete | 8086 | Trie + top-K frequency |
| 8 | news-feed | 8087 | Fan-out on write |
| 9 | web-crawler | 8088 | BFS + politeness + dedup |
| 10 | youtube | 8089 | Metadata + search + transcode |
| 11 | google-drive | 8090 | Files + folders + versioning |

## Memulai

```bash
go run ./services/url-shortener     # Pilih layanan mana pun
go test ./...              # 100 tes
go build ./...             # Build semua
```
