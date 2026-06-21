package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	stockKeyPrefix       = "inv:stock:%s:%s" // hub_id:sku
	reservationKeyPrefix = "inv:reservation:%s"
	defaultReservationTTL = 15 * time.Minute
)

var reserveScript = redis.NewScript(`
local stock_key = KEYS[1]
local reservation_key = KEYS[2]
local qty = tonumber(ARGV[1])
local reservation_id = ARGV[2]
local ttl = tonumber(ARGV[3])
local now = ARGV[4]
local user_id = ARGV[5]
local cart_id = ARGV[6]
local sku = ARGV[7]

local current = redis.call('GET', stock_key)
if current == false then
    return {-1, 0} -- SKU_NOT_FOUND
end

local available = tonumber(current)
if available < qty then
    return {-2, available} -- INSUFFICIENT_STOCK
end

redis.call('DECRBY', stock_key, qty)

redis.call('HSET', reservation_key,
    'reservation_id', reservation_id,
    'sku', sku,
    'quantity', qty,
    'user_id', user_id,
    'cart_id', cart_id,
    'status', 'RESERVED',
    'created_at', now,
    'expires_at', tostring(tonumber(now) + ttl * 1000)
)
redis.call('EXPIRE', reservation_key, ttl)

local remaining = tonumber(redis.call('GET', stock_key))
return {1, remaining}
`)

var releaseScript = redis.NewScript(`
local reservation_key = KEYS[1]
local stock_key = KEYS[2]

local data = redis.call('HGETALL', reservation_key)
if #data == 0 then
    return {-1} -- NOT_FOUND
end

local status = ''
local qty = 0
for i = 1, #data, 2 do
    if data[i] == 'status' then status = data[i+1] end
    if data[i] == 'quantity' then qty = tonumber(data[i+1]) end
end

if status ~= 'RESERVED' then
    return {-2} -- WRONG_STATUS
end

redis.call('HSET', reservation_key, 'status', 'RELEASED', 'released_at', ARGV[1])
redis.call('INCRBY', stock_key, qty)
return {1, qty}
`)

type InventoryRepo struct {
	rdb *redis.Client
}

func NewInventoryRepo(rdb *redis.Client) *InventoryRepo {
	return &InventoryRepo{rdb: rdb}
}

func (r *InventoryRepo) GetStock(ctx context.Context, hubID, sku string) (int, error) {
	key := fmt.Sprintf(stockKeyPrefix, hubID, sku)
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

func (r *InventoryRepo) SetStock(ctx context.Context, hubID, sku string, qty int) error {
	key := fmt.Sprintf(stockKeyPrefix, hubID, sku)
	return r.rdb.Set(ctx, key, qty, 0).Err()
}

func (r *InventoryRepo) Reserve(ctx context.Context, hubID, sku string, qty int, reservationID, userID, cartID string, ttl time.Duration) (int, error) {
	stockKey := fmt.Sprintf(stockKeyPrefix, hubID, sku)
	resKey := fmt.Sprintf(reservationKeyPrefix, reservationID)
	now := strconv.FormatInt(time.Now().Unix(), 10)

	result, err := reserveScript.Run(ctx, r.rdb, []string{stockKey, resKey},
		qty, reservationID, int(ttl.Seconds()), now, userID, cartID, sku).Slice()
	if err != nil {
		return 0, err
	}

	code := result[0].(int64)
	if code == -1 {
		return 0, fmt.Errorf("SKU_NOT_FOUND: %s", sku)
	}
	if code == -2 {
		remaining := result[1].(int64)
		return 0, fmt.Errorf("INSUFFICIENT_STOCK: have %d, need %d", remaining, qty)
	}
	return int(result[1].(int64)), nil
}

func (r *InventoryRepo) Release(ctx context.Context, reservationID, hubID, sku string) error {
	stockKey := fmt.Sprintf(stockKeyPrefix, hubID, sku)
	resKey := fmt.Sprintf(reservationKeyPrefix, reservationID)
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)

	result, err := releaseScript.Run(ctx, r.rdb, []string{resKey, stockKey}, now).Slice()
	if err != nil {
		return err
	}
	code := result[0].(int64)
	if code == -1 {
		return fmt.Errorf("RESERVATION_NOT_FOUND")
	}
	if code == -2 {
		return fmt.Errorf("WRONG_STATUS")
	}
	return nil
}

func (r *InventoryRepo) GetReservation(ctx context.Context, reservationID string) (map[string]string, error) {
	key := fmt.Sprintf(reservationKeyPrefix, reservationID)
	return r.rdb.HGetAll(ctx, key).Result()
}

func (r *InventoryRepo) ConfirmReservation(ctx context.Context, reservationID, hubID, sku string) error {
	key := fmt.Sprintf(reservationKeyPrefix, reservationID)
	data, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil || len(data) == 0 {
		return fmt.Errorf("reservation not found")
	}
	return r.rdb.HSet(ctx, key, "status", "CONFIRMED", "confirmed_at", time.Now().UnixMilli()).Err()
}

func (r *InventoryRepo) RunReaper(ctx context.Context) (int, error) {
	pattern := fmt.Sprintf(reservationKeyPrefix, "*")
	iter := r.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	released := 0

	for iter.Next(ctx) {
		key := iter.Val()
		parts := strings.Split(key, ":")
		if len(parts) < 3 {
			continue
		}
		data, _ := r.rdb.HGetAll(ctx, key).Result()
		status := data["status"]
		expiresStr := data["expires_at"]
		if status != "RESERVED" || expiresStr == "" {
			continue
		}
		expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil {
			continue
		}
		if time.Now().UnixMilli() > expiresAt+30000 { // 30s grace
			sku := data["sku"]
			qty, _ := strconv.Atoi(data["quantity"])
			hubID := parts[1] // simplified — need full parse
			stockKey := fmt.Sprintf(stockKeyPrefix, hubID, sku)
			r.rdb.IncrBy(ctx, stockKey, int64(qty))
			r.rdb.HSet(ctx, key, "status", "EXPIRED", "released_at", time.Now().UnixMilli())
			released++
		}
	}
	return released, nil
}
