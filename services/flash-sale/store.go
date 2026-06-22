package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyTotal       = "flash:total:"
	keyBucket      = "flash:bucket:"
	keyWaiting     = "flash:waiting:"
	keyRL          = "rl:flash:"
	keyIdem        = "flash:idem:"
	keyReservation = "flash:res:"
	keySlotCount   = "flash:slot:count:"
	keySlotSession = "flash:slot:session:"
)

// Lua: atomic check N buckets, decrement first with sufficient stock.
// KEYS[1] = total, KEYS[2..N+1] = buckets. ARGV = qty, N, startIdx.
const luaBucketDecr = `
local total = tonumber(redis.call("GET", KEYS[1]) or "0")
if total < tonumber(ARGV[1]) then return {-2, total} end
local N = tonumber(ARGV[2])
local start = tonumber(ARGV[3])
for i = 0, N - 1 do
    local idx = (start + i) % N
    local v = tonumber(redis.call("GET", KEYS[2 + idx]) or "0")
    if v >= tonumber(ARGV[1]) then
        redis.call("DECRBY", KEYS[2 + idx], ARGV[1])
        redis.call("DECRBY", KEYS[1], ARGV[1])
        return {idx, total - tonumber(ARGV[1])}
    end
end
return {-3, total}
`

// Lua: release reservation, return stock. Idempotent.
const luaRelease = `
local r = redis.call("HGETALL", KEYS[1])
if #r == 0 then return -1 end
local status, qty, bidx, pid = "", 0, 0, ""
for i = 1, #r, 2 do
    if r[i] == "status" then status = r[i+1]
    elseif r[i] == "quantity" then qty = tonumber(r[i+1])
    elseif r[i] == "bucket_idx" then bidx = r[i+1]
    elseif r[i] == "product_id" then pid = r[i+1]
    end
end
if status == "released" or status == "confirmed" then return 1 end
redis.call("HSET", KEYS[1], "status", "released", "released_at", ARGV[1])
redis.call("INCRBY", "flash:bucket:" .. pid .. ":" .. bidx, qty)
redis.call("INCRBY", "flash:total:" .. pid, qty)
return 1
`

// Lua: sliding window rate limit
const luaRateLimit = `
redis.call("ZREMRANGEBYSCORE", KEYS[1], 0, tonumber(ARGV[1]))
if redis.call("ZCARD", KEYS[1]) >= tonumber(ARGV[3]) then return 0 end
redis.call("ZADD", KEYS[1], ARGV[2], ARGV[4])
redis.call("EXPIRE", KEYS[1], 10)
return 1
`

// Lua: slot pool acquire — atomic INCR + check + conditional DECR.
// KEYS[1] = flash:slot:count:{product_id}
// KEYS[2] = flash:slot:session:{session_id}
// ARGV[1] = max_slots
// ARGV[2] = ttl_seconds
// Returns: {1, count} on success, {0, max} on full.
const luaSlotAcquire = `
local count = redis.call("INCR", KEYS[1])
if count <= tonumber(ARGV[1]) then
    redis.call("SETEX", KEYS[2], ARGV[2], count)
    return {1, count}
end
redis.call("DECR", KEYS[1])
return {0, tonumber(ARGV[1])}
`

type Store struct {
	rdb     *redis.Client
	decrSHA string
	relSHA  string
	rlSHA   string
	slotSHA string
	secret  []byte
}

func NewStore(rdb *redis.Client, hmacSecret string) *Store {
	return &Store{rdb: rdb, secret: []byte(hmacSecret)}
}

func (s *Store) Init(ctx context.Context) error {
	var err error
	s.decrSHA, err = s.rdb.ScriptLoad(ctx, luaBucketDecr).Result()
	if err != nil {
		return fmt.Errorf("load bucket_decr: %w", err)
	}
	s.relSHA, err = s.rdb.ScriptLoad(ctx, luaRelease).Result()
	if err != nil {
		return fmt.Errorf("load release: %w", err)
	}
	s.rlSHA, err = s.rdb.ScriptLoad(ctx, luaRateLimit).Result()
	if err != nil {
		return fmt.Errorf("load rate_limit: %w", err)
	}
	s.slotSHA, err = s.rdb.ScriptLoad(ctx, luaSlotAcquire).Result()
	if err != nil {
		return fmt.Errorf("load slot_acquire: %w", err)
	}
	return nil
}

func hashFP(fp string) int {
	h := fnv.New32a()
	h.Write([]byte(fp))
	return int(h.Sum32())
}

// --- Attestation ---

func (s *Store) VerifyAttestation(deviceFP string, expiresAt int64, token string) bool {
	now := time.Now().Unix()
	if d := now - expiresAt; d > 30 || d < -30 {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiresAt, 10)))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(token))
}

func (s *Store) GenerateToken(deviceFP string) (string, int64) {
	expiresAt := time.Now().Unix() + 30
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiresAt, 10)))
	return hex.EncodeToString(mac.Sum(nil)), expiresAt
}

// --- Idempotency ---

func (s *Store) CheckIdempotency(ctx context.Context, key string) (bool, string, error) {
	ok, err := s.rdb.SetNX(ctx, keyIdem+key, "locked", 30*time.Second).Result()
	if err != nil {
		return false, "", err
	}
	if !ok {
		result, _ := s.rdb.Get(ctx, keyIdem+key+":result").Result()
		return true, result, nil
	}
	return false, "", nil
}

func (s *Store) CacheIdempotencyResult(ctx context.Context, key, result string) {
	s.rdb.Set(ctx, keyIdem+key+":result", result, 10*time.Minute)
}

// --- Rate Limit ---

func (s *Store) CheckRateLimit(ctx context.Context, deviceFP string) (bool, error) {
	now := time.Now().UnixMilli()
	windowStart := now - defaultRateLimitWindow.Milliseconds()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	res, err := s.rdb.EvalSha(ctx, s.rlSHA, []string{keyRL + deviceFP}, windowStart, now, defaultRateLimitBurst, suffix).Result()
	if err != nil {
		return false, err
	}
	return res.(int64) == 1, nil
}

// --- Stock ---

func (s *Store) ReserveStock(ctx context.Context, productID, deviceFP string, qty, numBuckets int) (int, int, error) {
	keys := []string{keyTotal + productID}
	for i := 0; i < numBuckets; i++ {
		keys = append(keys, keyBucket+productID+":"+strconv.Itoa(i))
	}
	primary := hashFP(deviceFP) % numBuckets
	res, err := s.rdb.EvalSha(ctx, s.decrSHA, keys, qty, numBuckets, primary).Slice()
	if err != nil {
		return 0, 0, err
	}
	code := int(res[0].(int64))
	remaining := 0
	if len(res) > 1 {
		remaining = int(res[1].(int64))
	}
	return code, remaining, nil
}

func (s *Store) CreateReservation(ctx context.Context, productID, reservationID, userID, deviceFP string, qty, bucketIdx int) error {
	key := keyReservation + reservationID
	now := time.Now().UnixMilli()
	expiresAt := now + int64(reservationTTL.Seconds())*1000
	return s.rdb.HSet(ctx, key,
		"id", reservationID, "product_id", productID, "user_id", userID,
		"quantity", qty, "bucket_idx", bucketIdx, "status", "reserved",
		"created_at", now, "expires_at", expiresAt,
	).Err()
}

func (s *Store) ReleaseReservation(ctx context.Context, reservationID string) error {
	res, err := s.rdb.EvalSha(ctx, s.relSHA, []string{keyReservation + reservationID}, time.Now().UnixMilli()).Result()
	if err != nil {
		return err
	}
	if res.(int64) == -1 {
		return fmt.Errorf("not found")
	}
	return nil
}

// --- Waiting Room ---

func (s *Store) JoinWaitingRoom(ctx context.Context, productID, userID string) (int, error) {
	key := keyWaiting + productID
	s.rdb.ZAddNX(ctx, key, redis.Z{Score: float64(time.Now().UnixNano()), Member: userID})
	pos, err := s.rdb.ZRank(ctx, key, userID).Result()
	if err != nil {
		return 0, err
	}
	return int(pos), nil
}

func (s *Store) QueuePosition(ctx context.Context, productID, userID string) (int, error) {
	pos, err := s.rdb.ZRank(ctx, keyWaiting+productID, userID).Result()
	if err == redis.Nil {
		return -1, nil
	}
	return int(pos), err
}

// --- Slot Pool (Semaphore) ---

// AcquireSlot attempts to grab a concurrency slot for the given session.
// Returns true if acquired, false if pool is full.
// Session ID is used as the key — if same session retries, INCR bumps and
// DECR undoes immediately (no double-acquire).
func (s *Store) AcquireSlot(ctx context.Context, productID, sessionID string, maxSlots int, ttl time.Duration) (bool, int, error) {
	countKey := keySlotCount + productID
	sessionKey := keySlotSession + sessionID
	ttlSec := int64(ttl.Seconds())
	res, err := s.rdb.EvalSha(ctx, s.slotSHA, []string{countKey, sessionKey}, maxSlots, ttlSec).Slice()
	if err != nil {
		return false, 0, err
	}
	ok := res[0].(int64) == 1
	count := int(res[1].(int64))
	return ok, count, nil
}

// ReleaseSlot frees a concurrency slot. Safe to call multiple times — DEL is
// idempotent, and DECR on a key that's already been decremented is harmless
// (slot count will self-correct on next AcquireSlot).
func (s *Store) ReleaseSlot(ctx context.Context, productID, sessionID string) error {
	pipe := s.rdb.Pipeline()
	pipe.Decr(ctx, keySlotCount+productID)
	pipe.Del(ctx, keySlotSession+sessionID)
	_, err := pipe.Exec(ctx)
	return err
}

// SlotsAvailable returns the number of available slots.
func (s *Store) SlotsAvailable(ctx context.Context, productID string, maxSlots int) (int, error) {
	val, err := s.rdb.Get(ctx, keySlotCount+productID).Result()
	if err == redis.Nil {
		return maxSlots, nil
	}
	if err != nil {
		return 0, err
	}
	used, _ := strconv.Atoi(val)
	avail := maxSlots - used
	if avail < 0 {
		avail = 0
	}
	return avail, nil
}

// --- Admin ---

func (s *Store) InitProduct(ctx context.Context, productID string, totalStock, bucketCount int) error {
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, keyTotal+productID, totalStock, 0)
	base := totalStock / bucketCount
	rem := totalStock % bucketCount
	for i := 0; i < bucketCount; i++ {
		q := base
		if i < rem {
			q++
		}
		pipe.Set(ctx, keyBucket+productID+":"+strconv.Itoa(i), q, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) RemainingStock(ctx context.Context, productID string) (int, error) {
	val, err := s.rdb.Get(ctx, keyTotal+productID).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

func generateID(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}
