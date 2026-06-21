package kit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewServer_HealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv := NewServer(Config{Port: "8080", Env: "test"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Data == nil {
		t.Error("expected data in response")
	}
}

func TestGinMode(t *testing.T) {
	if m := ginMode("production"); m != gin.ReleaseMode {
		t.Errorf("expected release mode, got %s", m)
	}
	if m := ginMode("test"); m != gin.TestMode {
		t.Errorf("expected test mode, got %s", m)
	}
	if m := ginMode(""); m != gin.DebugMode {
		t.Errorf("expected debug mode, got %s", m)
	}
}
