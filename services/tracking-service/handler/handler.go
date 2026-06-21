package handler

import (
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/service"
	"github.com/gin-gonic/gin"
)

// Handler wires WebSocket and SSE endpoints to the Gin engine.
type Handler struct {
	Ingestor   *service.Ingestor
	SSEHandler *service.SSEHandler
}

// New creates a Handler with all dependencies wired.
func New(ing *service.Ingestor, sse *service.SSEHandler) *Handler {
	return &Handler{
		Ingestor:   ing,
		SSEHandler: sse,
	}
}

// RegisterRoutes adds tracking routes to the given Gin engine.
func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/ws/driver/location", h.WebSocketUpgrade)
	g.GET("/sse/order/tracking", h.SSEStream)
}

// WebSocketUpgrade upgrades to WebSocket for driver GPS ingestion.
func (h *Handler) WebSocketUpgrade(c *gin.Context) {
	h.Ingestor.HandleWebSocket(c.Writer, c.Request)
}

// SSEStream streams position updates to the user.
func (h *Handler) SSEStream(c *gin.Context) {
	h.SSEHandler.ServeHTTP(c.Writer, c.Request)
}

// HealthCheck returns service health.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "tracking-service"})
}
