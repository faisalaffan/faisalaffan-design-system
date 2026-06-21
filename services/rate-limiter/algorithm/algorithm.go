package algorithm

import "time"

type Algorithm interface {
	Allow(key string, limit int, window time.Duration) bool
}
