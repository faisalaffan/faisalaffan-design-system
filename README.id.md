# faisalaffan-design-system

<p align="center">
  <a href="README.md">🇬🇧 English</a>
</p>

<p align="center">
  <img src="assets/banner/banner.png" alt="banner" width="800"/>
</p>

<p align="center">
  <img src="assets/logo/logo.png" alt="faisalaffan-design-system logo" width="400"/>
</p>

---

Go monorepo mengimplementasikan semua 12 problem system design dari "System Design Interview" (2nd Ed) karya Alex Xu, didukung teori dari "Designing Data-Intensive Applications" karya Kleppmann.

**100 tes | 44 package | 12 layanan | 2 package bersama**

## Arsitektur

```mermaid
flowchart TB
    subgraph "Batch 1"
        US["url-shortener<br/>:8080"]
        RL["rate-limiter<br/>:8081"]
    end

    subgraph "Batch 2"
        ID["unique-id-generator<br/>:8084"]
        KV["key-value-store<br/>:8085"]
        AC["search-autocomplete<br/>:8086"]
    end

    subgraph "Batch 3"
        CS["chat-system<br/>:8082<br/>WebSocket"]
        NS["notification-system<br/>:8083<br/>pub/sub"]
    end

    subgraph "Batch 4"
        NF["news-feed<br/>:8087<br/>fan-out"]
        WC["web-crawler<br/>:8088<br/>BFS"]
    end

    subgraph "Batch 5"
        YT["youtube<br/>:8089"]
        GD["google-drive<br/>:8090"]
    end

    subgraph "Bersama (pkg/)"
        KIT["kit<br/>Gin factory"]
        CH["consistenthash<br/>Hash ring"]
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

    style KIT fill:#009688,color:#fff
    style CH fill:#ff9800,color:#000
```

## Layanan

| # | Layanan | Port | Pola Utama | Batch |
|---|---------|------|-----------|-------|
| 1 | **url-shortener** | 8080 | Base62 + collision retry | 1 |
| 2 | **rate-limiter** | 8081 | Sliding window + token bucket | 1 |
| 3 | **chat-system** | 8082 | WebSocket rooms + broadcast | 3 |
| 4 | **notification-system** | 8083 | Pub/sub + multi-channel | 3 |
| 5 | **unique-id-generator** | 8084 | Snowflake 64-bit | 2 |
| 6 | **key-value-store** | 8085 | Consistent hashing sharding | 2 |
| 7 | **search-autocomplete** | 8086 | Trie + top-K frekuensi | 2 |
| 8 | **news-feed** | 8087 | Fan-out on write | 4 |
| 9 | **web-crawler** | 8088 | BFS + politeness + dedup | 4 |
| 10 | **youtube** | 8089 | Metadata + search + transcode | 5 |
| 11 | **google-drive** | 8090 | File + folder + versioning | 5 |

## Menjalankan

```bash
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

go test ./...                   # 100 tes di 44 package
go build ./...                  # Build semua
go vet ./...                    # Vet semua
```

## Dependensi

```bash
docker compose up -d       # Redis + Postgres (untuk persistensi mendatang)
```

## Keputusan Teknis

| Keputusan | Alasan |
|-----------|--------|
| **Gin Gonic** | Framework HTTP Go tercepat. Ekosistem middleware kuat. Produktif tanpa korbankan performa. |
| **Interface-first storage** | Setiap layanan mendefinisikan interface `Storage`. Default in-memory, bisa ditukar ke Redis/Postgres tanpa ubah handler. |
| **Single `go.mod`** | Repo portfolio solo. Versi dependency bersama. Multi-module berlebihan untuk skala ini. |
| **Base62 random shortcode** | 7 karakter dari [a-zA-Z0-9] = ~3.5 triliun kombinasi. Acak + retry lebih simpel dari hash URL. |
| **Sliding window rate limiting** | Lebih presisi dari fixed window. Profil memori lebih baik dari token bucket untuk high-cardinality keys. |
| **Virtual nodes (150 replika)** | Consistent hashing dengan 150 virtual node. `crc32` untuk kecepatan — cukup uniform. |
| **Snowflake 64-bit ID** | Timestamp(41) + Worker(10) + Sequence(12) = 4096 ID/ms/worker, ~69 tahun masa pakai. Tanpa koordinasi. |
| **Trie-based autocomplete** | Pencarian prefix O(k). Top-K berdasarkan frekuensi + tiebreak leksikografis. Concurrent-safe untuk read. |
| **Fan-out on write** | Push post baru ke timeline semua follower saat write. Tradeoff write amplification untuk instant timeline reads. |
| **BFS crawler dengan politeness** | URL frontier berbasis channel. Delay per domain. Ekstraksi link HTML via `golang.org/x/net/html`. |
| **WebSocket rooms** | Room event loop berbasis goroutine (channel join/leave/broadcast). Riwayat pesan ring buffer (100 msg cap). |
| **Notifikasi multi-channel** | Interface Sender: in-app (tersimpan), email, push (simulasi). Pub/sub mendekopel publisher dari delivery. |
| **Transcoding simulasi** | Pemrosesan video async berbasis goroutine. State machine: uploading → processing → ready. |
| **File versioning** | Riwayat versi immutable per file. Setiap update menambah versi baru. Konten lama dipertahankan untuk rollback. |
| **Graceful shutdown** | Setiap layanan menangani SIGINT/SIGTERM dengan timeout 5 detik. Pola production di kode latihan. |
