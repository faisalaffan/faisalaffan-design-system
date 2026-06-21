package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/search-autocomplete/trie"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAutocompleteHandler(trie.New())
	h.Register(&r.RouterGroup)
	return r
}

func TestAutocompleteHandler_SearchEmpty(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/autocomplete?q=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAutocompleteHandler_TrainAndSearch(t *testing.T) {
	r := setupRouter()

	// Train
	body := `{"term":"golang"}`
	req, _ := http.NewRequest("POST", "/train", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("train failed: %d", w.Code)
	}

	// Search
	req, _ = http.NewRequest("GET", "/autocomplete?q=gol", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("search failed: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "golang") {
		t.Errorf("expected golang in results: %s", w.Body.String())
	}
}

func TestAutocompleteHandler_TrainBulk(t *testing.T) {
	r := setupRouter()
	body := `{"terms":["rust","ruby","react"]}`
	req, _ := http.NewRequest("POST", "/train/bulk", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("bulk train failed: %d", w.Code)
	}

	req, _ = http.NewRequest("GET", "/autocomplete?q=r", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "rust") {
		t.Errorf("expected rust in results: %s", w.Body.String())
	}
}

func TestAutocompleteHandler_TrainMissingTerm(t *testing.T) {
	r := setupRouter()
	body := `{}`
	req, _ := http.NewRequest("POST", "/train", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
