package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/web-crawler/crawler"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewCrawlHandler(crawler.New(time.Second))
	h.Register(&r.RouterGroup)
	return r
}

func TestCrawlHandler_StartCrawl(t *testing.T) {
	r := setupRouter()
	body := `{"url":"https://example.com","max_pages":5}`
	req, _ := http.NewRequest("POST", "/crawl", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "job_id") {
		t.Errorf("expected job_id in response: %s", w.Body.String())
	}
}

func TestCrawlHandler_GetJobNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/crawl/nonexist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCrawlHandler_InvalidURL(t *testing.T) {
	r := setupRouter()
	body := `{"url":"not-a-url"}`
	req, _ := http.NewRequest("POST", "/crawl", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
