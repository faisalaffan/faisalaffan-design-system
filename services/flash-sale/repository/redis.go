package repository

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	totalKeyPrefix      = "flash:sale:total:"
	bucketKeyPrefix     = "flash:sale:bucket:"
	waitingRoomKeyPrefix = "flash:sale:waiting:"
	rateLimitKeyPrefix  = "rl:flash:"
)

const bucketDecrSrc = `
local total = redis.call("GET", KEYS[1])
if not total then return -1 end
total = tonumber(total)
local qty = tonumber(ARGV[1])
if total < qty then return -2 end
local N = tonumber(ARGV[2])
local startIdx = tonumber(ARGV[3])
for i = 0, N - 1 do
    local idx = (startIdx + i) % N
    local bucket = tonumber(redis.call("GET", KEYS[2 + idx])) or 0
    if bucket >= qty then
        redis.call("DECRBY", KEYS[2 + idx], qty)
        redis.call("DECRBY", KEYS[1], qty)
        return idx
    end
end
return -3
`

const rateLimitSrc = `
local windowStart = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
redis.call("ZREMRANGEBYSCORE", KEYS[1], 0, windowStart)
local count = redis.call("ZCARD", KEYS[1])
if count >= burst then return 0 end
local member = tostring(now) .. ":" .. ARGV[4]
redis.call("ZADD", KEYS[1], now, member)
redis.call("EXPIRE", KEYS[1], 10)
return 1
`

type FlashSaleRepo struct {
	rdb     redis.UniversalClient
	decrSHA string
	rlSHA   string
}

func New(rdb redis.UniversalClient) *FlashSaleRepo {
	return &FlashSaleRepo{rdb: rdb}
}

func (r *FlashSaleRepo) Init(ctx context.Context) error {
	sha, err := r.rdb.ScriptLoad(ctx, bucketDecrSrc).Result()
	if err != nil {
		return fmt.Errorf("load bucket_decr: %w", err)
	}
	r.decrSHA = sha

	sha, err = r.rdb.ScriptLoad(ctx, rateLimitSrc).Result()
	if err != nil {
		return fmt.Errorf("load rate_limit: %w", err)
	}
	r.rlSHA = sha
	return nil
}

func hashDeviceFp(deviceFP string) int {
	h := fnv.New32a()
	h.Write([]byte(deviceFP))
	return int(h.Sum32())
}

func (r *FlashSaleRepo) totalKey(productID string) string {
	return totalKeyPrefix + productID
}

func (r *FlashSaleRepo) bucketKey(productID string, idx int) string {
	return bucketKeyPrefix + productID + ":" + strconv.Itoa(idx)
}

func (r *FlashSaleRepo) waitingRoomKey(productID string) string {
	return waitingRoomKeyPrefix + productID
}

func (r *FlashSaleRepo) rateLimitKey(deviceFP string) string {
	return rateLimitKeyPrefix + deviceFP
}

// BucketDecrement atomically deducts quantity from stock buckets.
// Returns the used bucket index (>=0) on success, or a negative error code:
//   -1 = product not found; -2 = sold out (total < qty); -3 = no single bucket with enough stock.
func (r *FlashSaleRepo) BucketDecrement(ctx context.Context, productID string, quantity, numBuckets int, deviceFP string) (int, error) {
	keys := make([]string, 0, numBuckets+1)
	keys = append(keys, r.totalKey(productID))
	for i := 0; i < numBuckets; i++ {
		keys = append(keys, r.bucketKey(productID, i))
	}
	primaryIdx := hashDeviceFp(deviceFP) % numBuckets

	res, err := r.rdb.EvalSha(ctx, r.decrSHA, keys, quantity, numBuckets, primaryIdx).Result()
	if err != nil {
		return 0, fmt.Errorf("bucket_decr evalsha: %w", err)
	}
	return int(res.(int64)), nil
}

// CheckRateLimit enforces a sliding-window rate limit per device fingerprint.
// Returns true if the request is allowed, false if rate-limited.
func (r *FlashSaleRepo) CheckRateLimit(ctx context.Context, deviceFP string, window time.Duration, burst int) (bool, error) {
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)

	res, err := r.rdb.EvalSha(ctx, r.rlSHA, []string{r.rateLimitKey(deviceFP)}, windowStart, now, burst, suffix).Result()
	if err != nil {
		return false, fmt.Errorf("rate_limit evalsha: %w", err)
	}
	return res.(int64) == 1, nil
}

// JoinWaitingRoom adds the user to the sorted-set waiting room.
// Returns the user's 0-based position (already in queue or freshly added).
func (r *FlashSaleRepo) JoinWaitingRoom(ctx context.Context, productID, userID string) (int, error) {
	key := r.waitingRoomKey(productID)
	now := float64(time.Now().UnixNano())

	n, err := r.rdb.ZAddNX(ctx, key, redis.Z{Score: now, Member: userID}).Result()
	if err != nil {
		return 0, fmt.Errorf("zaddnx waiting_room: %w", err)
	}
	_ = n // 1 if new, 0 if already present

	pos, err := r.rdb.ZRank(ctx, key, userID).Result()
	if err != nil {
		return 0, fmt.Errorf("zrank waiting_room: %w", err)
	}
	return int(pos), nil
}

// QueuePosition returns the 0-based position of the user, or -1 if not in the waiting room.
func (r *FlashSaleRepo) QueuePosition(ctx context.Context, productID, userID string) (int, error) {
	pos, err := r.rdb.ZRank(ctx, r.waitingRoomKey(productID), userID).Result()
	if err == redis.Nil {
		return -1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("zrank queue_position: %w", err)
	}
	return int(pos), nil
}

// InitProduct distributes totalStock evenly across bucketCount Redis keys and sets the total counter.
func (r *FlashSaleRepo) InitProduct(ctx context.Context, productID string, totalStock, bucketCount int) error {
	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, r.totalKey(productID), totalStock, 0)
	base := totalStock / bucketCount
	rem := totalStock % bucketCount
	for i := 0; i < bucketCount; i++ {
		s := base
		if i < rem {
			s++
		}
		pipe.Set(ctx, r.bucketKey(productID, i), s, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}
