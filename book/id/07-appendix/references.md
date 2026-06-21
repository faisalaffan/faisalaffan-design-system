# Referensi

Sumber daya berikut menginformasikan desain dan implementasi proyek ini.

## Buku

1. **Alex Xu** -- *System Design Interview: An Insider's Guide* (Edisi 2), 2021. Bab 4-15 menyediakan pernyataan masalah dan desain tingkat tinggi yang diimplementasikan proyek ini sebagai layanan Go yang berfungsi.

2. **Martin Kleppmann** -- *Designing Data-Intensive Applications: The Big Ideas Behind Reliable, Scalable, and Maintainable Systems*. O'Reilly Media, 2017. Teori latar belakang tentang model konsistensi, partisi, replikasi, dan trade-off sistem terdistribusi yang mendasari keputusan desain di setiap layanan.

## Dokumentasi Resmi

3. **Go Standard Library** -- <https://pkg.go.dev/std>. Digunakan secara ekstensif di semua layanan: `net/http`, `sync`, `encoding/json`, `container/heap` (untuk top-K di search-autocomplete), `crypto/sha256` (untuk deduplikasi di web-crawler), dan `hash/crc32` (untuk consistent hashing).

## Framework dan Pustaka

4. **Gin Gonic** -- <https://github.com/gin-gonic/gin>. Framework HTTP yang digunakan oleh semua layanan. Menyediakan routing, middleware chaining, request binding, dan rendering JSON.

5. **gorilla/websocket** -- <https://github.com/gorilla/websocket>. Implementasi WebSocket yang digunakan oleh chat system untuk koneksi dua arah yang persisten.

6. **golang.org/x/net/html** -- Pustaka ekstensi Go resmi untuk parsing HTML, digunakan oleh web crawler untuk ekstraksi tautan selama traversal BFS.

## Pola dan Algoritma

7. **Snowflake ID** -- Algoritma pembuatan ID unik terdistribusi Twitter. Dijelaskan dalam posting blog Snowflake (2010) dan banyak dirujuk di seluruh literatur desain sistem.

8. **Consistent Hashing** -- Awalnya dijelaskan oleh David Karger dkk. (1997) untuk digunakan dalam caching terdistribusi. Diperluas dengan virtual node dalam makalah Amazon Dynamo (2007).

9. **Sliding Window Algorithm** -- Pendekatan rate limiting yang dijelaskan dalam literatur desain sistem sebagai peningkatan dari penghitung fixed-window, menyediakan penegakan tingkat permintaan yang lebih halus.
