package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/chat-system/room"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewChatHandler(room.NewManager())
	h.Register(&r.RouterGroup)
	return r
}

func TestChatHandler_ListRooms(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/api/rooms", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestChatHandler_MessagesNotFound(t *testing.T) {
	r := setupRouter()
	req, _ := http.NewRequest("GET", "/api/rooms/nonexist/messages", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
