package store

import (
	"sort"
	"sync"
	"time"
)

type Post struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

type User struct {
	ID string `json:"id"`
}

type MemoryStore struct {
	mu       sync.RWMutex
	users    map[string]*User
	posts    map[string][]*Post     // userID -> posts
	follows  map[string]map[string]bool  // followerID -> set of followeeIDs
	timeline map[string][]*Post     // userID -> timeline (fan-out)
	postSeq  int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:    make(map[string]*User),
		posts:    make(map[string][]*Post),
		follows:  make(map[string]map[string]bool),
		timeline: make(map[string][]*Post),
	}
}

func (s *MemoryStore) CreateUser(id string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := &User{ID: id}
	s.users[id] = u
	s.posts[id] = make([]*Post, 0)
	s.timeline[id] = make([]*Post, 0)
	return u
}

func (s *MemoryStore) CreatePost(userID, content string) *Post {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.postSeq++
	p := &Post{
		ID:        userID + "_" + time.Now().Format("20060102150405") + "_" + content[:min(8, len(content))],
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now().UnixMilli(),
	}

	s.posts[userID] = append(s.posts[userID], p)

	// Fan-out: push to follower timelines
	for followerID := range s.follows {
		if s.follows[followerID][userID] {
			s.timeline[followerID] = append([]*Post{p}, s.timeline[followerID]...)
		}
	}
	// Also add to own timeline
	s.timeline[userID] = append([]*Post{p}, s.timeline[userID]...)

	return p
}

func (s *MemoryStore) Follow(followerID, followeeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[followerID]; !ok {
		return false
	}
	if _, ok := s.users[followeeID]; !ok {
		return false
	}
	if s.follows[followerID] == nil {
		s.follows[followerID] = make(map[string]bool)
	}
	s.follows[followerID][followeeID] = true
	return true
}

func (s *MemoryStore) GetTimeline(userID string, offset, limit int) []*Post {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tl := s.timeline[userID]
	if offset >= len(tl) {
		return []*Post{}
	}
	end := offset + limit
	if end > len(tl) {
		end = len(tl)
	}
	result := make([]*Post, end-offset)
	copy(result, tl[offset:end])
	return result
}

func (s *MemoryStore) GetUserPosts(userID string) []*Post {
	s.mu.RLock()
	defer s.mu.RUnlock()

	posts := s.posts[userID]
	result := make([]*Post, len(posts))
	copy(result, posts)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})
	return result
}
