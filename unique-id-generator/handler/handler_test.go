package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faisalaffan/faisalaffan-design-system/unique-id-generator/snowflake"
	"github.com/gin-gonic/gin"
)

func TestIDHandler_Generate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g, _ := snowflake.New(0)
	h := NewIDHandler(g)
	h.Register(&r.RouterGroup)

	req, _ := http.NewRequest("GET", "/id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
