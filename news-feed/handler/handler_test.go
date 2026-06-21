package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/news-feed/store"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewNewsFeedHandler(store.NewMemoryStore())
	h.Register(&r.RouterGroup)
	return r
}

func TestNewsFeedHandler_CreateUser(t *testing.T) {
	r := setupRouter()
	body := `{"id":"alice"}`
	req, _ := http.NewRequest("POST", "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestNewsFeedHandler_CreatePost(t *testing.T) {
	r := setupRouter()

	// Create user first
	req, _ := http.NewRequest("POST", "/users", strings.NewReader(`{"id":"bob"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Create post
	body := `{"user_id":"bob","content":"hello world"}`
	req, _ = http.NewRequest("POST", "/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNewsFeedHandler_Timeline(t *testing.T) {
	r := setupRouter()

	// Create user + post
	req, _ := http.NewRequest("POST", "/users", strings.NewReader(`{"id":"charlie"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	req, _ = http.NewRequest("POST", "/posts", strings.NewReader(`{"user_id":"charlie","content":"first post"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Get timeline
	req, _ = http.NewRequest("GET", "/timeline?user=charlie", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "first post") {
		t.Errorf("expected post in timeline: %s", w.Body.String())
	}
}

func TestNewsFeedHandler_FollowAndTimeline(t *testing.T) {
	r := setupRouter()

	// Create two users
	for _, u := range []string{`{"id":"dave"}`, `{"id":"eve"}`} {
		req, _ := http.NewRequest("POST", "/users", strings.NewReader(u))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	// dave follows eve
	req, _ := http.NewRequest("POST", "/follow", strings.NewReader(`{"follower_id":"dave","followee_id":"eve"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// eve posts
	req, _ = http.NewRequest("POST", "/posts", strings.NewReader(`{"user_id":"eve","content":"eve's post"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// dave's timeline should have eve's post (fan-out)
	req, _ = http.NewRequest("GET", "/timeline?user=dave", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data struct {
			Timeline []store.Post `json:"timeline"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data.Timeline) != 1 {
		t.Errorf("expected 1 post in dave's timeline, got %d", len(resp.Data.Timeline))
	}
}
