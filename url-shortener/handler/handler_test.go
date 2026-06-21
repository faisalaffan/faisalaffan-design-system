package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/service"
	"github.com/faisalaffan/faisalaffan-design-system/url-shortener/storage"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := storage.NewMemoryStore()
	svc := service.NewShortenService(store, "http://localhost:8080")
	h := NewShortenHandler(svc)
	h.Register(&r.RouterGroup)
	return r
}

func TestShortenHandler_Shorten(t *testing.T) {
	r := setupRouter()

	body := `{"url":"https://example.com/test"}`
	req, _ := http.NewRequest("POST", "/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "short_url") {
		t.Errorf("response missing short_url: %s", w.Body.String())
	}
}

func TestShortenHandler_LookupNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/nonexist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestShortenHandler_InvalidURL(t *testing.T) {
	r := setupRouter()
	body := `{"url":"not-a-url"}`
	req, _ := http.NewRequest("POST", "/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
