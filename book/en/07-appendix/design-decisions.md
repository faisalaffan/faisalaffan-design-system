# Design Decisions

Every architectural decision in this project was made with a specific trade-off in mind. This document captures the rationale behind each one, the alternatives considered, and why the chosen approach won.

---

## 1. Gin Gonic

**Decision:** Use Gin Gonic as the HTTP framework for all services.

**Rationale:** Gin Gonic offers the best balance of performance and developer ergonomics among Go HTTP frameworks. It sustains high throughput with minimal allocation overhead (comparable to raw `net/http` in many benchmarks) while providing middleware chaining, request binding, and route grouping out of the box. The middleware ecosystem allows cross-cutting concerns — recovery, logging, rate limiting, CORS — to be composed declaratively rather than duplicated per handler. Alternatives like `net/http`'s default mux were rejected for lacking parameterized routing and middleware support without third-party wrappers; frameworks like Echo or Fiber were considered but offered no meaningful advantage over Gin for this project's scope. Using a single framework across all 11 services ensures consistent patterns for error handling, request validation, and response formatting, which simplifies onboarding and reduces cognitive load when switching between services.

---

## 2. Interface-First Storage

**Decision:** Every service defines a `Storage` interface for its data layer, with an in-memory implementation as default.

**Rationale:** Abstracting storage behind an interface means the handler layer never depends on a specific database implementation. The in-memory default (backed by `sync.RWMutex`-protected maps) allows running every service with zero infrastructure dependencies — no Redis, no Postgres, no external databases. When a production-grade deployment is needed, a new implementation of the same interface (e.g., `redisStorage` or `postgresStorage`) can be swapped in without touching a single handler. This pattern also simplifies testing: test suites inject a fresh in-memory store per test case without mocking infrastructure. The trade-off is a thin indirection layer and slightly more code per service, but the decoupling justifies the cost at any scale beyond a throwaway prototype.

---

## 3. Single `go.mod`

**Decision:** One `go.mod` at the repository root — no multi-module workspace.

**Rationale:** Multi-module repositories solve version conflicts when different modules depend on different versions of the same dependency. This project has no such conflict: all 11 services and 2 shared packages use the same dependency versions. A single `go.mod` means one source of truth for dependency resolution, one `go.sum`, and no workspace configuration to maintain. `go build ./...` and `go test ./...` work immediately across the entire tree. If this project grows to include independently-versioned modules (e.g., a separate SDK or CLI tool), migrating to a multi-module workspace would be straightforward — but for a solo portfolio monorepo at this scale, single-module is the pragmatic choice.

---

## 4. Base62 Random Shortcode

**Decision:** Generate shortcodes as random 7-character Base62 strings with collision retry, rather than hashing the URL.

**Rationale:** A 7-character Base62 alphabet `[a-zA-Z0-9]` yields 62^7 = ~3.5 trillion unique codes, making collisions astronomically unlikely at any practical scale. The random generation approach is simpler than URL hashing: it avoids choosing a hash function, truncating to fit the code length, and handling hash collisions (which still need retry logic). It also eliminates the need to rehash when the same URL is shortened twice — each request gets a unique shortcode, which is often the desired behavior (tracking click sources, for example). The trade-off is that identical URLs produce different shortcodes, which wastes storage compared to a content-addressed approach, but this is negligible for a URL shortener's data footprint.

---

## 5. Sliding Window Rate Limiting

**Decision:** Implement rate limiting using a sliding window algorithm.

**Rationale:** Sliding window rate limiting addresses the boundary problem inherent in fixed-window algorithms. With a fixed window, a burst of requests at the end of one window and the start of the next can double the allowed throughput over a short interval. Sliding window tracks request timestamps within the current window fractionally, smoothing the boundary transition. Compared to token bucket — the other common candidate — sliding window uses less memory for high-cardinality keys (no need to maintain a token count and refill timer per key). The memory profile matters when rate limiting by user ID, IP, or API key in systems with millions of unique clients. The trade-off is slightly higher per-request CPU cost to prune expired timestamps, but this is negligible relative to the precision gained.

---

## 6. Virtual Nodes (150 Replicas)

**Decision:** Use consistent hashing with 150 virtual nodes per physical node, keyed by `crc32` checksum.

**Rationale:** Consistent hashing solves the re-sharding problem: when a node joins or leaves, only K/N keys need to move (where K is total keys and N is number of nodes), versus nearly all keys in a naive hash-mod-N scheme. Virtual nodes (also called replicas) distribute the hash ring more uniformly, preventing hot spots when nodes have heterogeneous capacity or when a small number of physical nodes would otherwise produce an uneven split. The 150-replica figure is a well-established heuristic from Amazon's Dynamo paper — high enough to give good uniformity, low enough to keep the ring metadata small. `crc32` was chosen over cryptographic hashes (SHA-256, MD5) for speed; it is not security-sensitive since the hash ring is an internal data structure. The trade-off is O(log N) lookup time (binary search on sorted ring) versus O(1) for direct hash-mod-N, but the ring is small enough (150 \* N entries) that this is irrelevant in practice.

---

## 7. Snowflake 64-Bit IDs

**Decision:** Generate unique 64-bit IDs using the Snowflake algorithm: 41-bit timestamp + 10-bit worker ID + 12-bit sequence.

**Rationale:** Snowflake IDs are time-sortable, unique without coordination, and fit in a 64-bit integer — a native type in Go and most databases. The bit layout produces ~69 years of IDs from a custom epoch, up to 1024 workers, and 4096 IDs per millisecond per worker. No external service (like a database sequence or ZooKeeper) is needed to generate them. The 64-bit size is smaller than UUIDs (128 bits, 36-character string representation), which matters for index size and storage efficiency. The trade-off is clock dependency: if a worker's clock drifts backward, IDs can collide. Standard mitigations include blocking until the clock catches up or using a ZooKeeper-based epoch. For this project's scope, the basic Snowflake implementation suffices.

---

## 8. Trie-Based Autocomplete

**Decision:** Implement search autocomplete using an in-memory trie with top-K frequency retrieval.

**Rationale:** A trie provides O(k) prefix search (where k is the prefix length), which is the theoretical lower bound for prefix-based lookup. Each node stores a sorted top-K list of completions, pre-computed from frequency data, so querying "top 5 results for prefix 'ap'" requires only traversing 'a' → 'p' and reading the cached list — no sorting or ranking at query time. This makes it suitable for latency-sensitive autocomplete where every keystroke triggers a request. Concurrent reads are safe via `sync.RWMutex`, allowing many parallel queries while writes acquire the write lock during re-indexing. The trade-off is memory: a trie with fine-grained nodes can be large. Compact trie variants (radix tree, DAWG) could reduce memory at the cost of implementation complexity, but for the dataset sizes this project targets, the simple trie is sufficient.

---

## 9. Fan-Out on Write

**Decision:** Push new posts to all followers' timelines at write time (fan-out on write).

**Rationale:** Fan-out on write (push model) trades write amplification for instant timeline reads. When a user posts, the system iterates over the user's followers and inserts the post into each follower's timeline. Reading a timeline is then a simple O(1) lookup — fetch the pre-computed list. This is the right choice for celebrities with millions of followers; for them, a hybrid approach (push to active followers, pull from inactive) or pure pull model is more appropriate. For this project's scale, the simple push model demonstrates the core trade-off clearly: fast reads cost expensive writes. The alternative (fan-out on read / pull model) would require merging timelines from followed users at query time, shifting the cost to reads.

---

## 10. BFS Crawler with Politeness

**Decision:** Implement the web crawler as a breadth-first traversal over a channel-based URL frontier, with per-domain rate limiting for politeness.

**Rationale:** BFS ensures coverage breadth: the crawler discovers pages layer by layer rather than diving deep into a single domain. The URL frontier is implemented as Go channels, providing natural concurrency control — workers consume from the channel and send discovered URLs back. Per-domain delay enforces politeness: after fetching a page from `example.com`, the crawler waits a configurable interval before fetching another from the same domain. This respects `robots.txt` directives and prevents overloading any single origin server. HTML link extraction uses `golang.org/x/net/html`, the standard Go library for HTML parsing. Deduplication via a Bloom filter or set ensures each URL is crawled at most once per run. The trade-off is that BFS can consume significant memory for the frontier queue on large crawls, but controlling crawl depth mitigates this.

---

## 11. WebSocket Rooms

**Decision:** Implement chat rooms as goroutine-based event loops with a ring buffer for message history.

**Rationale:** Each chat room is a goroutine running an event loop that listens on three channels: `join`, `leave`, and `broadcast`. This model maps naturally to WebSocket chat semantics — users join rooms, send messages, and receive broadcasts. The goroutine-per-room approach is efficient in Go: goroutines are lightweight (~2 KB stack) and idle ones consume negligible resources. A ring buffer capped at 100 messages stores recent history, so newly joined users see recent messages without querying a database. The ring buffer is fixed-size and lock-free within the single goroutine, avoiding synchronization overhead. The alternative — a shared data structure with mutexes for all rooms — would couple room state management and reduce clarity. The trade-off is that a very large number of idle rooms could consume goroutine overhead, but practical deployments rarely have millions of concurrently active rooms.

---

## 12. Multi-Channel Notification

**Decision:** Implement notifications through a sender interface with separate implementations for in-app, email, and push notifications, decoupled via pub/sub.

**Rationale:** The sender interface (`Sender`) defines a single contract: `Send(recipient, title, body)`. Each channel implements it independently — in-app (persisted to storage, polled by the client), email (logs to stdout as simulation), and push (logs to stdout as simulation). Publishers (services that generate notifications) never know about delivery mechanisms; they publish to a channel, and registered senders consume from it. This pub/sub decoupling means adding a new channel (SMS, Slack, webhook) requires zero changes to publishers — just write a new `Sender` and register it. The trade-off is eventual delivery semantics: if a sender is slow or fails, the pub/sub channel must handle backpressure. For this project, the channel is buffered and synchronous, which is adequate for demonstration purposes.

---

## 13. Simulated Transcoding

**Decision:** Simulate video transcoding as an asynchronous goroutine-based process with a state machine.

**Rationale:** Video transcoding is computationally expensive and inherently asynchronous. This implementation models the real-world pipeline as a state machine: `uploading` → `processing` → `ready`. When a video is uploaded, a goroutine is spawned that simulates transcoding work (via `time.Sleep`), then transitions the video to `ready`. This mirrors production transcoding pipelines (AWS Elastic Transcoder, FFmpeg jobs) without requiring actual FFmpeg binaries or GPU hardware. The state machine pattern makes it easy to extend: adding a `failed` state, progress reporting, or parallel quality variants are just additional states and transitions. The trade-off is that simulated work is unrealistically predictable — real transcoding times vary with video length, resolution, and codec — but this is acceptable for demonstrating the async processing pattern and API design.

---

## 14. File Versioning

**Decision:** Store immutable version history for files — each update appends a new version rather than overwriting in place.

**Rationale:** Immutable versioning means every file update creates a new immutable snapshot of the file content, with an incrementing version number. Old versions are retained and accessible by version ID, enabling rollback, audit trails, and concurrent access to historical states. This is the same design used by Google Drive, Dropbox, and S3 object versioning. The implementation stores versions in an ordered slice per file metadata entry; the latest version is always the last element. The trade-off is storage amplification: editing a 100 MB file 50 times consumes ~5 GB of raw storage. Production systems use delta encoding or snapshot scheduling to mitigate this, but for this project, full-version storage is the clearest demonstration of the concept and is acceptable at the target data scale.

---

## 15. Graceful Shutdown

**Decision:** Every service handles SIGINT and SIGTERM with a 5-second graceful shutdown timeout.

**Rationale:** A service that dies without draining inflight requests can corrupt data, drop messages, or leave clients hanging. The graceful shutdown pattern — catching OS signals, notifying the HTTP server to stop accepting new requests, waiting for active requests to finish (with a timeout), then exiting — ensures clean teardown. The 5-second timeout is a reasonable default: long enough to complete typical HTTP requests (which should take milliseconds), short enough that the process won't hang indefinitely on a stuck handler. This pattern is implemented once in `pkg/kit` and reused by every service, ensuring consistent behavior across the project. The alternative — ignoring signals or calling `os.Exit(0)` immediately — is appropriate only for stateless batch jobs, not network services.
