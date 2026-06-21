package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/services/youtube/store"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewYouTubeHandler(store.NewMemoryStore())
	h.Register(&r.RouterGroup)
	return r
}

func TestYouTubeHandler_Upload(t *testing.T) {
	r := setupRouter()
	body := `{"title":"My Video","description":"A test","tags":["go","tutorial"]}`
	req, _ := http.NewRequest("POST", "/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "My Video") {
		t.Errorf("expected title in response: %s", w.Body.String())
	}
}

func TestYouTubeHandler_Get(t *testing.T) {
	r := setupRouter()

	// Upload first
	body := `{"title":"Test"}`
	req, _ := http.NewRequest("POST", "/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Get
	req, _ = http.NewRequest("GET", "/videos/vid_1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestYouTubeHandler_GetNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/videos/nonexist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestYouTubeHandler_List(t *testing.T) {
	r := setupRouter()

	for _, title := range []string{"A", "B"} {
		body := `{"title":"` + title + `"}`
		req, _ := http.NewRequest("POST", "/videos", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	req, _ := http.NewRequest("GET", "/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestYouTubeHandler_Search(t *testing.T) {
	r := setupRouter()

	body := `{"title":"Golang Tutorial","tags":["go"]}`
	req, _ := http.NewRequest("POST", "/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	req, _ = http.NewRequest("GET", "/search?q=golang", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Golang Tutorial") {
		t.Errorf("expected search result: %s", w.Body.String())
	}
}
