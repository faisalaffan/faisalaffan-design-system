package shard

import (
	"context"
	"testing"
)

func TestManager_PutGet(t *testing.T) {
	m := NewManager([]string{"shard-0", "shard-1", "shard-2"})
	ctx := context.Background()

	m.Put(ctx, "hello", "world")
	v, err := m.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if v != "world" {
		t.Errorf("expected world, got %s", v)
	}
}

func TestManager_Delete(t *testing.T) {
	m := NewManager([]string{"shard-0"})
	ctx := context.Background()

	m.Put(ctx, "key", "val")
	m.Delete(ctx, "key")
	_, err := m.Get(ctx, "key")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestManager_ShardDistribution(t *testing.T) {
	m := NewManager([]string{"a", "b", "c"})
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		m.Put(ctx, string(rune('A'+i%26))+string(rune('0'+i%10)), "v")
	}

	// Verify we can read back all keys
	for i := 0; i < 100; i++ {
		key := string(rune('A'+i%26)) + string(rune('0'+i%10))
		v, err := m.Get(ctx, key)
		if err != nil {
			t.Errorf("key %s not found: %v", key, err)
		}
		if v != "v" {
			t.Errorf("expected 'v', got %s", v)
		}
	}
}

func TestNodesFromEnv(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 1},
		{"a,b,c", 3},
		{"  x , y , z  ", 3},
	}

	for _, tc := range tests {
		nodes := NodesFromEnv(tc.input)
		if len(nodes) != tc.expected {
			t.Errorf("NodesFromEnv(%q): expected %d nodes, got %d", tc.input, tc.expected, len(nodes))
		}
	}
}
