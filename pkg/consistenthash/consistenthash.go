package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type HashRing struct {
	replicas int
	keys     []int
	hashMap  map[int]string
}

func New(replicas int) *HashRing {
	if replicas <= 0 {
		replicas = 150
	}
	return &HashRing{
		replicas: replicas,
		hashMap:  make(map[int]string),
	}
}

func (h *HashRing) Add(node string) {
	for i := 0; i < h.replicas; i++ {
		hash := int(crc32.ChecksumIEEE([]byte(strconv.Itoa(i) + node)))
		h.keys = append(h.keys, hash)
		h.hashMap[hash] = node
	}
	sort.Ints(h.keys)
}

func (h *HashRing) Remove(node string) {
	for i := 0; i < h.replicas; i++ {
		hash := int(crc32.ChecksumIEEE([]byte(strconv.Itoa(i) + node)))
		delete(h.hashMap, hash)
	}
	var newKeys []int
	for _, k := range h.keys {
		if _, ok := h.hashMap[k]; ok {
			newKeys = append(newKeys, k)
		}
	}
	h.keys = newKeys
	sort.Ints(h.keys)
}

func (h *HashRing) Get(key string) string {
	if len(h.keys) == 0 {
		panic("hash ring is empty")
	}
	hash := int(crc32.ChecksumIEEE([]byte(key)))
	idx := sort.Search(len(h.keys), func(i int) bool {
		return h.keys[i] >= hash
	})
	if idx == len(h.keys) {
		idx = 0
	}
	return h.hashMap[h.keys[idx]]
}
