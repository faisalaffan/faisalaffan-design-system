package storage

import "context"

type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key string, value string) error
	Delete(ctx context.Context, key string) error
}
