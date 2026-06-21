package service

import (
	"sort"
	"sync"

	"github.com/faisalaffan/faisalaffan-design-system/services/search-service/model"
)

// trieNode is a single node in the prefix trie.
type trieNode struct {
	children map[rune]*trieNode
	freq     int
}

// autocompleteTrie is a concurrency-safe prefix trie for fast autocomplete
// lookups. Each insertion increments a frequency counter at the terminal
// node, and search returns top-K results by frequency.
type autocompleteTrie struct {
	mu   sync.RWMutex
	root *trieNode
}

func newAutocompleteTrie() *autocompleteTrie {
	return &autocompleteTrie{root: &trieNode{children: make(map[rune]*trieNode)}}
}

// Insert adds term to the trie, incrementing its frequency by freq.
func (t *autocompleteTrie) Insert(term string, freq int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cur := t.root
	for _, ch := range term {
		if cur.children[ch] == nil {
			cur.children[ch] = &trieNode{children: make(map[rune]*trieNode)}
		}
		cur = cur.children[ch]
	}
	cur.freq += freq
}

// Search returns up to limit entries whose key shares the given prefix,
// sorted by frequency descending (then lexicographically as tie-breaker).
func (t *autocompleteTrie) Search(prefix string, limit int) []model.Suggestion {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cur := t.root
	for _, ch := range prefix {
		if cur.children[ch] == nil {
			return nil
		}
		cur = cur.children[ch]
	}

	var results []model.Suggestion
	t.collect(cur, prefix, &results)

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Text < results[j].Text
		}
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// collect performs a DFS from n, appending every terminal entry.
func (t *autocompleteTrie) collect(n *trieNode, prefix string, results *[]model.Suggestion) {
	if n.freq > 0 {
		*results = append(*results, model.Suggestion{Text: prefix, Score: n.freq})
	}
	for ch, child := range n.children {
		t.collect(child, prefix+string(ch), results)
	}
}
