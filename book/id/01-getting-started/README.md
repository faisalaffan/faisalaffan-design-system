# Memulai

Selamat datang di dokumentasi **faisalaffan-design-system**. Monorepo ini mengimplementasikan semua 12 masalah sistem desain dari buku Alex Xu *System Design Interview* (Ed. 2) sebagai layanan Go yang berfungsi, didasari oleh teori sistem terdistribusi dari Martin Kleppmann dalam *Designing Data-Intensive Applications*.

**100 tes | 44 paket | 11 layanan | 2 paket bersama**

## Apa yang Dicakup Proyek Ini

Setiap layanan menangani masalah desain sistem dunia nyata -- URL shortening, rate limiting, chat, notifikasi, ID terdistribusi, penyimpanan key-value, autocomplete, news feed, web crawling, pemrosesan video, dan penyimpanan file. Setiap masalah diimplementasikan dari awal dengan binary Go mandiri, menggunakan gaya arsitektur yang konsisten di seluruh monorepo.

Proyek ini diorganisir ke dalam lima batch, masing-masing mengelompokkan layanan terkait:

| # | Layanan | Port | Pola Inti | Batch |
|---|---------|------|-------------|-------|
| 1 | **url-shortener** | 8080 | Base62 + collision retry | 1 -- Inti |
| 2 | **rate-limiter** | 8081 | Sliding window + token bucket | 1 -- Inti |
| 3 | **chat-system** | 8082 | WebSocket rooms + broadcast | 3 -- Waktu Nyata |
| 4 | **notification-system** | 8083 | Pub/sub + multi-channel | 3 -- Waktu Nyata |
| 5 | **unique-id-generator** | 8084 | Snowflake 64-bit | 2 -- Data |
| 6 | **key-value-store** | 8085 | Consistent hashing sharding | 2 -- Data |
| 7 | **search-autocomplete** | 8086 | Trie + top-K frequency | 2 -- Data |
| 8 | **news-feed** | 8087 | Fan-out on write | 4 -- Skala |
| 9 | **web-crawler** | 8088 | BFS + politeness + dedup | 4 -- Skala |
| 10 | **youtube** | 8089 | Metadata + search + transcode | 5 -- Penyimpanan |
| 11 | **google-drive** | 8090 | Files + folders + versioning | 5 -- Penyimpanan |

## Cara Navigasi

| Bagian | Isi |
|---------|----------|
| **01 -- Memulai** | Ikhtisar arsitektur dan instruksi menjalankan (bagian ini) |
| **02 -- Batch 1: Inti** | Paket bersama (kit, consistenthash), URL shortener, rate limiter |
| **03 -- Batch 2: Data** | Unique ID generator, key-value store, search autocomplete |
| **04 -- Batch 3: Waktu Nyata** | Chat system, notification system |
| **05 -- Batch 4: Skala** | News feed, web crawler |
| **06 -- Batch 5: Penyimpanan** | YouTube, Google Drive |
| **07 -- Lampiran** | Keputusan teknis dan referensi |

## Prinsip Desain

- **Penyimpanan interface-first** -- Setiap layanan mendefinisikan interface `Storage`. In-memory sebagai default, dapat diganti ke Redis atau Postgres tanpa menyentuh handler.
- **`go.mod` tunggal** -- Satu graph dependensi, satu `go.sum`, tanpa konfigurasi workspace.
- **Pola konsisten** -- Framework HTTP Gin Gonic, graceful shutdown, penanganan error terstruktur -- diterapkan secara seragam di semua layanan.
- **Cakupan tes** -- 100 tes di 44 paket, menguji jalur sukses dan kasus tepi.

## Memulai

```bash
# Jalankan layanan apa pun
go run ./services/url-shortener

# Jalankan semua tes
go test ./...

# Build semua
go build ./...
```
