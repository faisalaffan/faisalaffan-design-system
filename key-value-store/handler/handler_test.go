package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/key-value-store/shard"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mgr := shard.NewManager([]string{"default"})
	h := NewKVHandler(mgr)
	h.Register(&r.RouterGroup)
	return r
}

func TestKVHandler_PutGet(t *testing.T) {
	r := setupRouter()

	req, _ := http.NewRequest("PUT", "/mykey", strings.NewReader("myvalue"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d", w.Code)
	}

	req, _ = http.NewRequest("GET", "/mykey", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "myvalue") {
		t.Errorf("expected myvalue in response: %s", w.Body.String())
	}
}

func TestKVHandler_GetNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestKVHandler_Delete(t *testing.T) {
	r := setupRouter()

	req, _ := http.NewRequest("PUT", "/temp", strings.NewReader("x"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	req, _ = http.NewRequest("DELETE", "/temp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("DELETE expected 200, got %d", w.Code)
	}

	req, _ = http.NewRequest("GET", "/temp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}
