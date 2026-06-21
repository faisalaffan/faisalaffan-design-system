package shard

import (
	"context"
	"strings"

	"github.com/faisalaffan/faisalaffan-design-system/key-value-store/storage"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/consistenthash"
)

type Manager struct {
	ring   *consistenthash.HashRing
	shards map[string]storage.Store
}

func NewManager(nodes []string) *Manager {
	ring := consistenthash.New(150)
	shards := make(map[string]storage.Store)
	for _, n := range nodes {
		ring.Add(n)
		shards[n] = storage.NewMemoryStore()
	}
	return &Manager{ring: ring, shards: shards}
}

func (m *Manager) getShard(key string) storage.Store {
	node := m.ring.Get(key)
	return m.shards[node]
}

func (m *Manager) Get(ctx context.Context, key string) (string, error) {
	return m.getShard(key).Get(ctx, key)
}

func (m *Manager) Put(ctx context.Context, key string, value string) error {
	return m.getShard(key).Put(ctx, key, value)
}

func (m *Manager) Delete(ctx context.Context, key string) error {
	return m.getShard(key).Delete(ctx, key)
}

func NodesFromEnv(val string) []string {
	if val == "" {
		return []string{"default"}
	}
	nodes := strings.Split(val, ",")
	for i := range nodes {
		nodes[i] = strings.TrimSpace(nodes[i])
	}
	if len(nodes) == 0 {
		return []string{"default"}
	}
	return nodes
}
