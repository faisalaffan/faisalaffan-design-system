package trie

import (
	"sort"
	"sync"
)

type node struct {
	children map[rune]*node
	freq     int
}

type Trie struct {
	mu   sync.RWMutex
	root *node
}

func New() *Trie {
	return &Trie{root: &node{children: make(map[rune]*node)}}
}

func (t *Trie) Insert(term string, freq int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cur := t.root
	for _, ch := range term {
		if cur.children[ch] == nil {
			cur.children[ch] = &node{children: make(map[rune]*node)}
		}
		cur = cur.children[ch]
	}
	cur.freq += freq
}

func (t *Trie) Increment(term string) {
	t.Insert(term, 1)
}

type Result struct {
	Term string `json:"term"`
	Freq int    `json:"freq"`
}

func (t *Trie) Search(prefix string, limit int) []Result {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cur := t.root
	for _, ch := range prefix {
		if cur.children[ch] == nil {
			return nil
		}
		cur = cur.children[ch]
	}

	var results []Result
	collect(cur, prefix, &results)

	sort.Slice(results, func(i, j int) bool {
		if results[i].Freq == results[j].Freq {
			return results[i].Term < results[j].Term
		}
		return results[i].Freq > results[j].Freq
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func collect(n *node, prefix string, results *[]Result) {
	if n.freq > 0 {
		*results = append(*results, Result{Term: prefix, Freq: n.freq})
	}
	for ch, child := range n.children {
		collect(child, prefix+string(ch), results)
	}
}
