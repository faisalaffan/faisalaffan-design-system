# Changelog

## 2026-06-22 — Initial Implementation

### Batch 1: Core + Rate Limiter + URL Shortener
- `feat(kit):` Gin server factory, config, response helpers, errors, middleware
- `feat(consistenthash):` Hash ring with 150 virtual nodes
- `feat(rate-limiter):` Token bucket, sliding window, fixed window algorithms
- `feat(url-shortener):` Base62 random shortcode, service layer, handler

### Batch 2: ID Generator + KV Store + Autocomplete
- `feat(unique-id-generator):` Snowflake 64-bit ID generator
- `feat(key-value-store):` Distributed KV with consistent hashing sharding
- `feat(search-autocomplete):` Trie-based autocomplete with top-K search

### Batch 3: Chat + Notifications
- `feat(chat-system):` WebSocket rooms with goroutine-based broadcast
- `feat(notification-system):` Pub/sub with multi-channel delivery

### Batch 4: News Feed + Web Crawler
- `feat(news-feed):` Fan-out on write timeline system
- `feat(web-crawler):` BFS crawler with politeness and deduplication

### Batch 5: YouTube + Google Drive
- `feat(youtube):` Video metadata, search, simulated transcoding
- `feat(google-drive):` File storage with folders, sharing, version history
