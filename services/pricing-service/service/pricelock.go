package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// PriceLockService persists locked delivery fees in Redis with a 15-minute TTL.
// All operations are fail-open: errors return zero values.
type PriceLockService struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewPriceLockService creates a new PriceLockService.
func NewPriceLockService(rdb *redis.Client) *PriceLockService {
	return &PriceLockService{
		rdb: rdb,
		ttl: 15 * time.Minute,
	}
}

// Set persists a fee lock with a 15-minute TTL. Fail-open: errors are returned
// so the caller can decide how to handle them.
func (s *PriceLockService) Set(ctx context.Context, orderID string, fee float64) error {
	return s.rdb.Set(ctx, lockKey(orderID), fee, s.ttl).Err()
}

// Get retrieves a locked fee. Returns (0, error) if not found or Redis is down.
// Callers should treat errors as "lock not found" and proceed without the lock.
func (s *PriceLockService) Get(ctx context.Context, orderID string) (float64, error) {
	val, err := s.rdb.Get(ctx, lockKey(orderID)).Float64()
	if err == redis.Nil {
		return 0, fmt.Errorf("price lock not found for order %s", orderID)
	}
	return val, err
}

func lockKey(orderID string) string {
	return "pricelock:" + orderID
}
