package consistenthash

import (
	"fmt"
	"testing"
)

func TestHashRing_AddGet(t *testing.T) {
	h := New(150)
	nodes := []string{"node-a", "node-b", "node-c"}
	for _, n := range nodes {
		h.Add(n)
	}

	results := make(map[string]int)
	for i := 0; i < 1000; i++ {
		node := h.Get(fmt.Sprintf("key-%d", i))
		results[node]++
	}

	for _, n := range nodes {
		if results[n] == 0 {
			t.Errorf("node %s got zero keys", n)
		}
	}
}

func TestHashRing_Remove(t *testing.T) {
	h := New(150)
	h.Add("node-a")
	h.Add("node-b")
	h.Remove("node-a")

	for i := 0; i < 100; i++ {
		if node := h.Get(fmt.Sprintf("key-%d", i)); node != "node-b" {
			t.Errorf("expected node-b, got %s after removing node-a", node)
		}
	}
}

func TestHashRing_Empty(t *testing.T) {
	h := New(150)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when getting from empty ring")
		}
	}()
	h.Get("any-key")
}

func TestHashRing_Distribution(t *testing.T) {
	h := New(150)
	for i := 0; i < 5; i++ {
		h.Add(fmt.Sprintf("node-%d", i))
	}

	counts := make(map[string]int)
	const keys = 100000
	for i := 0; i < keys; i++ {
		counts[h.Get(fmt.Sprintf("k-%d", i))]++
	}

	avg := keys / 5
	for node, count := range counts {
		diff := float64(count-avg) / float64(avg)
		if diff > 0.20 || diff < -0.20 {
			t.Errorf("node %s: %d keys (%.1f%% off avg)", node, count, diff*100)
		}
	}
}
