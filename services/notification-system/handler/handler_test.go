package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/services/notification-system/channel"
	"github.com/faisalaffan/faisalaffan-design-system/services/notification-system/service"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	store := channel.NewStore()
	h := NewNotificationHandler(service.New(store))
	h.Register(&r.RouterGroup)
	return r
}

func TestNotificationHandler_Send(t *testing.T) {
	r := setupRouter()
	body := `{"user_id":"user-1","channel":"in_app","title":"Hello","body":"Test notification"}`
	req, _ := http.NewRequest("POST", "/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Hello") {
		t.Errorf("expected title in response: %s", w.Body.String())
	}
}

func TestNotificationHandler_SendEmail(t *testing.T) {
	r := setupRouter()
	body := `{"user_id":"user-2","channel":"email","title":"Alert","body":"Server is down"}`
	req, _ := http.NewRequest("POST", "/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestNotificationHandler_GetByUser(t *testing.T) {
	r := setupRouter()

	// Send one first
	body := `{"user_id":"user-3","channel":"in_app","title":"Hi","body":"msg"}`
	req, _ := http.NewRequest("POST", "/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Get notifications
	req, _ = http.NewRequest("GET", "/notifications?user=user-3", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Hi") {
		t.Errorf("expected notification in response: %s", w.Body.String())
	}
}

func TestNotificationHandler_GetByIDNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/notifications/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestNotificationHandler_GetByUserMissingQuery(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/notifications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
