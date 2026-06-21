package handler

import (
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/service"
	"github.com/gin-gonic/gin"
)

// ETAHandler exposes the ETA estimation endpoints.
type ETAHandler struct {
	svc *service.ETAService
}

// NewETAHandler creates a new ETAHandler.
func NewETAHandler(svc *service.ETAService) *ETAHandler {
	return &ETAHandler{svc: svc}
}

// CalculateETA handles POST /eta/calculate.
func (h *ETAHandler) CalculateETA(c *gin.Context) {
	var req model.ETARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request: order_id, hub_id, item_count, hub_lat, hub_lng, cust_lat, cust_lng, queue_depth, drivers_avail required")
		return
	}

	resp, err := h.svc.Calculate(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, kit.Response{Error: err.Error()})
		return
	}
	kit.OK(c, resp)
}

// GetETA handles GET /eta/:order_id.
func (h *ETAHandler) GetETA(c *gin.Context) {
	orderID := c.Param("order_id")
	resp, err := h.svc.GetETA(c.Request.Context(), orderID)
	if err != nil {
		kit.NotFound(c, "ETA not found")
		return
	}
	kit.OK(c, resp)
}

// Register routes the handler onto a Gin router group.
func (h *ETAHandler) Register(r *gin.RouterGroup) {
	r.POST("/eta/calculate", h.CalculateETA)
	r.GET("/eta/:order_id", h.GetETA)
}
