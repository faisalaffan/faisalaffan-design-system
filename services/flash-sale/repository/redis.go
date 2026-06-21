package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/redis/go-redis/v9"
)

const (
	totalKeyPrefix       = "flash:sale:total:"
	bucketKeyPrefix      = "flash:sale:bucket:"
	waitingRoomKeyPrefix = "flash:sale:waiting:"
	rateLimitKeyPrefix   = "rl:flash:"
	idempotencyKeyPrefix = "flash:idem:"
	reservationKeyPrefix = "flash:reservation:"
)

// Lua: atomic bucket decrement → creates reservation with TTL
const bucketDecrSrc = `
local total = redis.call("GET", KEYS[1])
if not total then return {-1, 0, ""} end
total = tonumber(total)
local qty = tonumber(ARGV[1])
if total < qty then return {-2, 0, ""} end
local N = tonumber(ARGV[2])
local startIdx = tonumber(ARGV[3])
for i = 0, N - 1 do
    local idx = (startIdx + i) % N
    local bucket = tonumber(redis.call("GET", KEYS[2 + idx])) or 0
    if bucket >= qty then
        redis.call("DECRBY", KEYS[2 + idx], qty)
        redis.call("DECRBY", KEYS[1], qty)
        -- store reservation with TTL
        local resKey = ARGV[5]
        local now = ARGV[6]
        local ttl = tonumber(ARGV[7])
        local expiresAt = tostring(now + ttl * 1000)
        redis.call("HSET", resKey,
            "id", ARGV[4], "product_id", ARGV[8], "user_id", ARGV[9],
            "quantity", qty, "bucket_idx", idx, "status", "reserved",
            "created_at", now, "expires_at", expiresAt)
        redis.call("EXPIRE", resKey, ttl)
        return {idx, total - qty, ARGV[4]}
    end
end
return {-3, 0, ""}
`

// Lua: release reservation + return stock (idempotent)
const releaseReservationSrc = `
local res = redis.call("HGETALL", KEYS[1])
if #res == 0 then return -1 end
local status, qty, bucketIdx, productID, expiresAt = "", 0, 0, "", ""
for i = 1, #res, 2 do
    if res[i] == "status" then status = res[i+1]
    elseif res[i] == "quantity" then qty = tonumber(res[i+1])
    elseif res[i] == "bucket_idx" then bucketIdx = tonumber(res[i+1])
    elseif res[i] == "product_id" then productID = res[i+1]
    elseif res[i] == "expires_at" then expiresAt = res[i+1]
    end
end
if status ~= "reserved" and status ~= "expired" then return -2 end
if status == "released" or status == "confirmed" then return 1 end -- idempotent

redis.call("HSET", KEYS[1], "status", "released", "released_at", ARGV[1])
local bucketKey = "flash:sale:bucket:" .. productID .. ":" .. tostring(bucketIdx)
local totalKey = "flash:sale:total:" .. productID
redis.call("INCRBY", bucketKey, qty)
redis.call("INCRBY", totalKey, qty)
return 1
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
	rdb      redis.UniversalClient
	decrSHA  string
	relSHA   string
	rlSHA    string
}

func New(rdb redis.UniversalClient) *FlashSaleRepo {
	return &FlashSaleRepo{rdb: rdb}
}

func (r *FlashSaleRepo) Init(ctx context.Context) error {
	var err error
	r.decrSHA, err = r.rdb.ScriptLoad(ctx, bucketDecrSrc).Result()
	if err != nil {
		return fmt.Errorf("load bucket_decr: %w", err)
	}
	r.relSHA, err = r.rdb.ScriptLoad(ctx, releaseReservationSrc).Result()
	if err != nil {
		return fmt.Errorf("load release_reservation: %w", err)
	}
	r.rlSHA, err = r.rdb.ScriptLoad(ctx, rateLimitSrc).Result()
	if err != nil {
		return fmt.Errorf("load rate_limit: %w", err)
	}
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

func (r *FlashSaleRepo) idempotencyKey(idemKey string) string {
	return idempotencyKeyPrefix + idemKey
}

func (r *FlashSaleRepo) reservationKey(id string) string {
	return reservationKeyPrefix + id
}

// CheckIdempotency returns true if this idempotency key has already been processed.
// Sets a lock with TTL to prevent concurrent duplicate requests.
func (r *FlashSaleRepo) CheckIdempotency(ctx context.Context, idemKey string) (bool, string, error) {
	key := r.idempotencyKey(idemKey)
	// Try to set lock
	ok, err := r.rdb.SetNX(ctx, key, "locked", 30*time.Second).Result()
	if err != nil {
		return false, "", fmt.Errorf("idempotency check: %w", err)
	}
	if !ok {
		// Already processed or in-flight — return stored result if any
		result, _ := r.rdb.Get(ctx, key+":result").Result()
		return true, result, nil
	}
	return false, "", nil
}

// StoreIdempotencyResult saves the checkout result for future duplicate requests.
func (r *FlashSaleRepo) StoreIdempotencyResult(ctx context.Context, idemKey, result string) error {
	key := r.idempotencyKey(idemKey) + ":result"
	return r.rdb.Set(ctx, key, result, 10*time.Minute).Err()
}

// BucketDecrement atomically deducts stock and creates a reservation with TTL.
// Returns (bucketIdx, remaining, reservationID) or error codes: -1 not found, -2 sold out, -3 fragmented.
func (r *FlashSaleRepo) BucketDecrement(ctx context.Context, productID, reservationID string, quantity, numBuckets int, deviceFP, userID string, ttl time.Duration) (int, int, string, error) {
	keys := make([]string, 0, numBuckets+1)
	keys = append(keys, r.totalKey(productID))
	for i := 0; i < numBuckets; i++ {
		keys = append(keys, r.bucketKey(productID, i))
	}
	primaryIdx := hashDeviceFp(deviceFP) % numBuckets
	now := time.Now().UnixMilli()

	res, err := r.rdb.EvalSha(ctx, r.decrSHA, keys,
		quantity, numBuckets, primaryIdx,
		reservationID, r.reservationKey(reservationID), now, int(ttl.Seconds()),
		productID, userID,
	).Slice()
	if err != nil {
		return 0, 0, "", fmt.Errorf("bucket_decr evalsha: %w", err)
	}

	code := int(res[0].(int64))
	if code < 0 {
		return code, 0, "", nil
	}
	remaining := int(res[1].(int64))
	rid := res[2].(string)
	return code, remaining, rid, nil
}

// ReleaseReservation returns stock and marks reservation as released.
func (r *FlashSaleRepo) ReleaseReservation(ctx context.Context, reservationID string) error {
	res, err := r.rdb.EvalSha(ctx, r.relSHA, []string{r.reservationKey(reservationID)}, time.Now().UnixMilli()).Result()
	if err != nil {
		return fmt.Errorf("release evalsha: %w", err)
	}
	code := res.(int64)
	if code == -1 {
		return fmt.Errorf("reservation not found")
	}
	if code == -2 {
		return fmt.Errorf("cannot release in current status")
	}
	return nil
}

// ConfirmReservation marks reservation as confirmed.
func (r *FlashSaleRepo) ConfirmReservation(ctx context.Context, reservationID string) error {
	key := r.reservationKey(reservationID)
	status, err := r.rdb.HGet(ctx, key, "status").Result()
	if err == redis.Nil {
		return fmt.Errorf("reservation not found")
	}
	if err != nil {
		return err
	}
	if status != "reserved" {
		return fmt.Errorf("cannot confirm in status %s", status)
	}
	return r.rdb.HSet(ctx, key, "status", "confirmed", "confirmed_at", time.Now().UnixMilli()).Err()
}

// RunReaper scans for expired reservations and releases stock.
func (r *FlashSaleRepo) RunReaper(ctx context.Context, productID string, gracePeriod time.Duration) (int, error) {
	pattern := reservationKeyPrefix + "*"
	iter := r.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	released := 0
	now := time.Now().UnixMilli()

	for iter.Next(ctx) {
		key := iter.Val()
		data, err := r.rdb.HGetAll(ctx, key).Result()
		if err != nil || len(data) == 0 {
			continue
		}
		if data["status"] != "reserved" {
			continue
		}
		expiresStr := data["expires_at"]
		if expiresStr == "" {
			continue
		}
		expiresAt, _ := strconv.ParseInt(expiresStr, 10, 64)
		if now > expiresAt+gracePeriod.Milliseconds() {
			r.ReleaseReservation(ctx, data["id"])
			released++
		}
	}
	return released, nil
}

// CleanWaitingRoom removes stale entries older than TTL.
func (r *FlashSaleRepo) CleanWaitingRoom(ctx context.Context, productID string, ttl time.Duration) (int, error) {
	key := r.waitingRoomKey(productID)
	cutoff := float64(time.Now().Add(-ttl).UnixNano())
	return int(r.rdb.ZRemRangeByScore(ctx, key, "0", strconv.FormatFloat(cutoff, 'f', 0, 64)).Val()), nil
}

// AdmitNext admits the next batch from the waiting room.
func (r *FlashSaleRepo) AdmitNext(ctx context.Context, productID string, batchSize int) ([]string, error) {
	key := r.waitingRoomKey(productID)
	members, err := r.rdb.ZPopMin(ctx, key, int64(batchSize)).Result()
	if err != nil {
		return nil, err
	}
	users := make([]string, 0, len(members))
	for _, m := range members {
		users = append(users, m.Member.(string))
	}
	return users, nil
}

// CheckRateLimit sliding window per device fingerprint.
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

// JoinWaitingRoom adds user to sorted set, returns 0-based position.
func (r *FlashSaleRepo) JoinWaitingRoom(ctx context.Context, productID, userID string) (int, error) {
	key := r.waitingRoomKey(productID)
	now := float64(time.Now().UnixNano())
	r.rdb.ZAddNX(ctx, key, redis.Z{Score: now, Member: userID})
	pos, err := r.rdb.ZRank(ctx, key, userID).Result()
	if err != nil {
		return 0, fmt.Errorf("zrank waiting_room: %w", err)
	}
	return int(pos), nil
}

// QueuePosition returns 0-based position, -1 if not in queue.
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

// InitProduct distributes stock across buckets.
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

// GetStockForDryRun reads the current total stock for a product without decrementing.
// This is a read-only operation used by the dry-run pipeline.
func (r *FlashSaleRepo) GetStockForDryRun(ctx context.Context, productID string) (int, error) {
	key := r.totalKey(productID)
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

// PublishQueueEvent publishes a queue position update to Redis pub/sub (for SSE).
func (r *FlashSaleRepo) PublishQueueEvent(ctx context.Context, productID, userID string, event model.QueueEvent) error {
	b, _ := json.Marshal(event)
	return r.rdb.Publish(ctx, "flash:queue:"+productID+":"+userID, b).Err()
}

// SubscribeQueue returns a pub/sub channel for queue events.
func (r *FlashSaleRepo) SubscribeQueue(ctx context.Context, productID, userID string) *redis.PubSub {
	return r.rdb.Subscribe(ctx, "flash:queue:"+productID+":"+userID)
}
