package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// Redis counter repository — atomic Lua scripts
// ---------------------------------------------------------------------------

// RedisCounter provides atomic usage-limit checks and budget reservations
// using Redis Lua scripts. All keys are prefixed with "promo:".
type RedisCounter struct {
	client *redis.Client
	prefix string
}

// LimitResult carries the outcome of a limit check.
type LimitResult struct {
	Allowed  bool   `json:"allowed"`
	Error    string `json:"error,omitempty"`    // human readable
	ErrorCode string `json:"error_code,omitempty"` // machine code
	Current  int64  `json:"current,omitempty"`
}

const (
	ErrCodeOK        = "OK"
	ErrCodeTotalHit  = "TOTAL_LIMIT"
	ErrCodeDailyHit  = "DAILY_LIMIT"
	ErrCodeUserHit   = "USER_LIMIT"
	ErrCodeBudgetCap = "BUDGET_CAP"
)

// NewRedisCounter creates a new counter repository backed by the given Redis client.
func NewRedisCounter(client *redis.Client) *RedisCounter {
	return &RedisCounter{
		client: client,
		prefix: "promo",
	}
}

// ---------------------------------------------------------------------------
// Lua: incrementAndCheck
//
// Increments total counter if below limit.
// Returns: status (OK | TOTAL_LIMIT | DAILY_LIMIT | USER_LIMIT), current_total
//
// Keys: promo:{ruleID}:total, promo:{ruleID}:daily:{date}, promo:{ruleID}:user:{userID}
// Args: max_total, max_daily, max_per_user, TTL seconds for daily & user keys
// ---------------------------------------------------------------------------

const scriptIncrCheck = `
local key_total   = KEYS[1]
local key_daily   = KEYS[2]
local key_user    = KEYS[3]
local max_total   = tonumber(ARGV[1])
local max_daily   = tonumber(ARGV[2])
local max_user    = tonumber(ARGV[3])
local ttl_seconds = tonumber(ARGV[4])

-- check total
local cur_total = redis.call("GET", key_total)
if cur_total and tonumber(cur_total) >= max_total then
	return {0, "TOTAL_LIMIT", cur_total}
end
-- check daily
local cur_daily = redis.call("GET", key_daily)
if cur_daily and tonumber(cur_daily) >= max_daily then
	return {0, "DAILY_LIMIT", cur_daily}
end
-- check user
local cur_user = redis.call("GET", key_user)
if cur_user and tonumber(cur_user) >= max_user then
	return {0, "USER_LIMIT", cur_user}
end

-- increment all three atomically
local new_total = redis.call("INCR", key_total)
local new_daily = redis.call("INCR", key_daily)
local new_user  = redis.call("INCR", key_user)

-- set expiry on first creation (when value is 1)
if new_total == 1 then
	redis.call("EXPIRE", key_total, 86400 * 365) -- 1 year
end
if new_daily == 1 then
	redis.call("EXPIRE", key_daily, ttl_seconds)
end
if new_user == 1 then
	redis.call("EXPIRE", key_user, ttl_seconds)
end

return {1, "OK", new_total}
`

// IncrementAndCheck atomically increments total+daily+user counters;
// returns OK only if all three are below their respective limits.
func (rc *RedisCounter) IncrementAndCheck(ctx context.Context, ruleID, userID string, limits LimitParams) LimitResult {
	if !limits.HasAny() {
		return LimitResult{Allowed: true, ErrorCode: ErrCodeOK}
	}

	today := time.Now().UTC().Format("2006-01-02")
	// remaining TTL for daily and user keys (end of day)
	now := time.Now().UTC()
	tomorrow := now.Truncate(24*time.Hour).Add(25 * time.Hour)
	ttl := int(tomorrow.Sub(now).Seconds())
	if ttl < 60 {
		ttl = 60
	}

	keys := []string{
		rc.key("counter", ruleID, "total"),
		rc.key("counter", ruleID, "daily", today),
		rc.key("counter", ruleID, "user", userID),
	}
	args := []interface{}{
		limits.MaxTotal,
		limits.MaxDaily,
		limits.MaxPerUser,
		ttl,
	}

	result, err := rc.client.Eval(ctx, scriptIncrCheck, keys, args...).Result()
	if err != nil {
		return LimitResult{Allowed: false, ErrorCode: ErrCodeTotalHit, Error: err.Error()}
	}

	resp := result.([]interface{})
	allowed := resp[0].(int64) == 1
	code := resp[1].(string)
	cur, _ := resp[2].(int64)

	return LimitResult{
		Allowed:   allowed,
		ErrorCode: code,
		Current:   cur,
	}
}

// ---------------------------------------------------------------------------
// Lua: checkAllLimits — read-only check (no increment)
// Returns: {code, current_total, current_daily, current_user}
// ---------------------------------------------------------------------------

const scriptCheckAll = `
local key_total   = KEYS[1]
local key_daily   = KEYS[2]
local key_user    = KEYS[3]
local max_total   = tonumber(ARGV[1])
local max_daily   = tonumber(ARGV[2])
local max_user    = tonumber(ARGV[3])

local cur_total = redis.call("GET", key_total) or 0
local cur_daily = redis.call("GET", key_daily) or 0
local cur_user  = redis.call("GET", key_user) or 0

if tonumber(cur_total) >= max_total then
	return {"TOTAL_LIMIT", cur_total, cur_daily, cur_user}
end
if tonumber(cur_daily) >= max_daily then
	return {"DAILY_LIMIT", cur_total, cur_daily, cur_user}
end
if tonumber(cur_user) >= max_user then
	return {"USER_LIMIT", cur_total, cur_daily, cur_user}
end
return {"OK", cur_total, cur_daily, cur_user}
`

// CheckAllLimits reads total, daily, and user counters without incrementing.
// Use this before rendering UI to show remaining usage.
func (rc *RedisCounter) CheckAllLimits(ctx context.Context, ruleID, userID string, limits LimitParams) LimitResult {
	if !limits.HasAny() {
		return LimitResult{Allowed: true, ErrorCode: ErrCodeOK}
	}

	today := time.Now().UTC().Format("2006-01-02")
	keys := []string{
		rc.key("counter", ruleID, "total"),
		rc.key("counter", ruleID, "daily", today),
		rc.key("counter", ruleID, "user", userID),
	}
	args := []interface{}{limits.MaxTotal, limits.MaxDaily, limits.MaxPerUser}

	result, err := rc.client.Eval(ctx, scriptCheckAll, keys, args...).Result()
	if err != nil {
		return LimitResult{Allowed: false, ErrorCode: ErrCodeTotalHit, Error: err.Error()}
	}

	resp := result.([]interface{})
	code := resp[0].(string)
	total, _ := resp[1].(int64)
	daily, _ := resp[2].(int64)
	user, _ := resp[3].(int64)

	return LimitResult{
		Allowed:   code == ErrCodeOK,
		ErrorCode: code,
		Current:   total + daily + user, // best-effort sum
	}
}

// ---------------------------------------------------------------------------
// Lua: budgetReserve — atomic INCRBY with cap check
// ---------------------------------------------------------------------------

const scriptBudgetReserve = `
local key_budget  = KEYS[1]
local amount      = tonumber(ARGV[1])
local cap         = tonumber(ARGV[2])

local used = redis.call("GET", key_budget) or 0
local new_used = tonumber(used) + amount

if cap > 0 and new_used > cap then
	local remaining = cap - tonumber(used)
	if remaining < 0 then remaining = 0 end
	return {0, "BUDGET_CAP", remaining}
end

redis.call("SET", key_budget, new_used)
redis.call("EXPIRE", key_budget, 86400 * 365)
return {1, "OK", new_used}
`

// BudgetReserve atomically reserves `amount` from the budget cap.
// Returns the remaining budget if capped, or an error code.
func (rc *RedisCounter) BudgetReserve(ctx context.Context, ruleID string, amount, cap float64) LimitResult {
	if cap <= 0 {
		return LimitResult{Allowed: true, ErrorCode: ErrCodeOK}
	}

	keys := []string{rc.key("budget", ruleID)}
	args := []interface{}{int64(amount * 100), int64(cap * 100)} // work in cents

	result, err := rc.client.Eval(ctx, scriptBudgetReserve, keys, args...).Result()
	if err != nil {
		return LimitResult{Allowed: false, ErrorCode: ErrCodeBudgetCap, Error: err.Error()}
	}

	resp := result.([]interface{})
	allowed := resp[0].(int64) == 1
	code := resp[1].(string)
	remaining, _ := resp[2].(int64)

	return LimitResult{
		Allowed:   allowed,
		ErrorCode: code,
		Current:   remaining,
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (rc *RedisCounter) key(parts ...string) string {
	// prefix + "/" + parts joined by ":"
	joined := rc.prefix
	for _, p := range parts {
		joined += ":" + p
	}
	return joined
}

// LimitParams is a subset of model.UsageLimits for the Redis interface.
type LimitParams struct {
	MaxTotal   int
	MaxDaily   int
	MaxPerUser int
}

func (l LimitParams) HasAny() bool {
	return l.MaxTotal > 0 || l.MaxDaily > 0 || l.MaxPerUser > 0
}

// Ensure fmt imported.
var _ = fmt.Sprintf
