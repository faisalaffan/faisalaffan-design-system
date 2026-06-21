package storage

import "time"

type Store interface {
	Increment(key string, window time.Duration) (int, error)
	Count(key string, window time.Duration) (int, error)
	Reset(key string) error
}
