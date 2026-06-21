package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/services/google-drive/store"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewDriveHandler(store.NewMemoryStore())
	h.Register(&r.RouterGroup)
	return r
}

func TestDriveHandler_CreateFile(t *testing.T) {
	r := setupRouter()
	body := `test content`
	req, _ := http.NewRequest("POST", "/files?name=test.txt&owner_id=user1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDriveHandler_GetFile(t *testing.T) {
	r := setupRouter()
	body := `hello`
	req, _ := http.NewRequest("POST", "/files?name=f.txt&owner_id=u1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	req, _ = http.NewRequest("GET", "/files/file_1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDriveHandler_UpdateFile(t *testing.T) {
	r := setupRouter()
	body := `v1`
	req, _ := http.NewRequest("POST", "/files?name=f.txt&owner_id=u1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body2 := `v2 updated`
	req, _ = http.NewRequest("PUT", "/files/file_1", strings.NewReader(body2))
	req.Header.Set("Content-Type", "text/plain")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDriveHandler_CreateFolder(t *testing.T) {
	r := setupRouter()
	body := `{"name":"docs","owner_id":"user1"}`
	req, _ := http.NewRequest("POST", "/folders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestDriveHandler_ShareFile(t *testing.T) {
	r := setupRouter()
	body := `data`
	req, _ := http.NewRequest("POST", "/files?name=shared.txt&owner_id=u1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	share := `{"user_id":"u2","permission":"read"}`
	req, _ = http.NewRequest("POST", "/files/file_1/share", strings.NewReader(share))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
