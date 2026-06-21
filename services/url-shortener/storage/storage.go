package storage

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("shortcode not found")

type Storage interface {
	Save(ctx context.Context, code string, url string) error
	Get(ctx context.Context, code string) (string, error)
	Exists(ctx context.Context, code string) (bool, error)
}
