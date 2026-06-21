package store

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

type Video struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"` // uploading, processing, ready, failed
	FileSize    int64    `json:"file_size"`
	Duration    int      `json:"duration_secs"`
	ViewCount   int      `json:"view_count"`
	UploadedAt  int64    `json:"uploaded_at"`
}

type MemoryStore struct {
	mu     sync.RWMutex
	videos map[string]*Video
	seq    int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{videos: make(map[string]*Video)}
}

func (s *MemoryStore) Create(title, description string, tags []string) *Video {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	v := &Video{
		ID:          fmt.Sprintf("vid_%d", s.seq),
		Title:       title,
		Description: description,
		Tags:        tags,
		Status:      "uploading",
		UploadedAt:  time.Now().UnixMilli(),
	}

	s.videos[v.ID] = v

	// Simulate transcoding after "upload"
	go s.transcode(v.ID)
	return v
}

func (s *MemoryStore) transcode(id string) {
	time.Sleep(1 * time.Second) // simulate processing time

	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.videos[id]; ok {
		v.Status = "ready"
		v.Duration = rand.Intn(600) + 10 // 10-610 seconds
	}
}

func (s *MemoryStore) Get(id string) *Video {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.videos[id]
}

func (s *MemoryStore) IncrementView(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.videos[id]; ok {
		v.ViewCount++
	}
}

func (s *MemoryStore) List() []*Video {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Video, 0, len(s.videos))
	for _, v := range s.videos {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UploadedAt > result[j].UploadedAt
	})
	return result
}

func (s *MemoryStore) Search(query string) []*Video {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(query)
	var result []*Video
	for _, v := range s.videos {
		if strings.Contains(strings.ToLower(v.Title), q) ||
			strings.Contains(strings.ToLower(v.Description), q) ||
			containsTag(v.Tags, q) {
			result = append(result, v)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UploadedAt > result[j].UploadedAt
	})
	return result
}

func containsTag(tags []string, q string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}
