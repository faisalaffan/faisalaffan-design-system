# References

The following resources informed the design and implementation of this project.

## Books

1. **Alex Xu** — *System Design Interview: An Insider's Guide* (2nd Edition), 2021. Chapters 4-15 provide the problem statements and high-level designs that this project implements as working Go services.

2. **Martin Kleppmann** — *Designing Data-Intensive Applications: The Big Ideas Behind Reliable, Scalable, and Maintainable Systems*. O'Reilly Media, 2017. Background theory on consistency models, partitioning, replication, and distributed systems trade-offs that underlie the design decisions in each service.

## Official Documentation

3. **Go Standard Library** — <https://pkg.go.dev/std>. Used extensively across all services: `net/http`, `sync`, `encoding/json`, `container/heap` (for top-K in search-autocomplete), `crypto/sha256` (for deduplication in web-crawler), and `hash/crc32` (for consistent hashing).

## Frameworks and Libraries

4. **Gin Gonic** — <https://github.com/gin-gonic/gin>. HTTP framework used by all services. Provides routing, middleware chaining, request binding, and JSON rendering.

5. **gorilla/websocket** — <https://github.com/gorilla/websocket>. WebSocket implementation used by the chat system for persistent bidirectional connections.

6. **golang.org/x/net/html** — Official Go extended library for HTML parsing, used by the web crawler for link extraction during BFS traversal.

## Patterns and Algorithms

7. **Snowflake ID** — Twitter's distributed unique ID generation algorithm. Described in the Snowflake blog post (2010) and widely referenced across system design literature.

8. **Consistent Hashing** — Originally described by David Karger et al. (1997) for use in distributed caching. Extended with virtual nodes in the Amazon Dynamo paper (2007).

9. **Sliding Window Algorithm** — Rate limiting approach described in the system design literature as an improvement over fixed-window counters, providing smoother request rate enforcement.
