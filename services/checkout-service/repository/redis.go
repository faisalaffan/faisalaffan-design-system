package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	idempotencyLockPrefix = "idem:lock:"
	idempotencyResultPrefix = "idem:result:"
	webhookPrefix          = "webhook:payment:"
	lockTTL                = 5 * time.Second
	resultTTL              = 24 * time.Hour
)

// RedisClient wraps a redis.Client for idempotency and deduplication.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new wrapper. If client is nil, operations are
// no-ops so the service can run without Redis in development.
func NewRedisClient(client *redis.Client) *RedisClient {
	return &RedisClient{client: client}
}

// TryLock attempts to acquire an idempotency lock for the given key.
// Returns true if the lock was acquired (first request).
// Returns false if the lock already exists (duplicate request).
// On Redis errors, the lock is considered not acquired so the caller can
// proceed with the request (fail-open).
func (r *RedisClient) TryLock(ctx context.Context, key string) bool {
	if r.client == nil {
		return true
	}
	ok, err := r.client.SetNX(ctx, idempotencyLockPrefix+key, "locked", lockTTL).Result()
	if err != nil {
		log.Printf("redis: TryLock error for key=%s: %v", key, err)
		return true // fail-open: let the request through
	}
	return ok
}

// StoreResult caches the result of an idempotent operation.
func (r *RedisClient) StoreResult(ctx context.Context, key string, result interface{}) {
	if r.client == nil {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("redis: StoreResult marshal error for key=%s: %v", key, err)
		return
	}
	if err := r.client.Set(ctx, idempotencyResultPrefix+key, data, resultTTL).Err(); err != nil {
		log.Printf("redis: StoreResult set error for key=%s: %v", key, err)
	}
}

// GetResult retrieves a cached idempotent result. Returns nil if not found.
func (r *RedisClient) GetResult(ctx context.Context, key string) interface{} {
	if r.client == nil {
		return nil
	}
	data, err := r.client.Get(ctx, idempotencyResultPrefix+key).Bytes()
	if err != nil {
		if err != redis.Nil {
			log.Printf("redis: GetResult error for key=%s: %v", key, err)
		}
		return nil
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("redis: GetResult unmarshal error for key=%s: %v", key, err)
		return nil
	}
	return result
}

// MarkWebhookProcessed records that a webhook has been processed.
// Returns true if this is the first time (lock acquired).
func (r *RedisClient) MarkWebhookProcessed(ctx context.Context, txID string) bool {
	if r.client == nil {
		return true
	}
	ok, err := r.client.SetNX(ctx, webhookPrefix+txID, "processed", resultTTL).Result()
	if err != nil {
		log.Printf("redis: MarkWebhookProcessed error for txID=%s: %v", txID, err)
		return true
	}
	return ok
}

// IsWebhookProcessed checks if a webhook has already been processed.
func (r *RedisClient) IsWebhookProcessed(ctx context.Context, txID string) bool {
	if r.client == nil {
		return false
	}
	exists, err := r.client.Exists(ctx, webhookPrefix+txID).Result()
	if err != nil {
		log.Printf("redis: IsWebhookProcessed error for txID=%s: %v", txID, err)
		return false
	}
	return exists == 1
}

// HealthCheck pings Redis and returns an error if unreachable.
func (r *RedisClient) HealthCheck(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return r.client.Ping(ctx).Err()
}
