package handler

import (
	"context"
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/model"
	"github.com/gin-gonic/gin"
)

// PricingOrchestrator defines the interface the handler needs from the
// pricing service layer.
type PricingOrchestrator interface {
	Calculate(ctx context.Context, req *model.FeeRequest) (*model.FeeResponse, error)
	GetLockedPrice(ctx context.Context, orderID string) (float64, error)
	GetSignals(ctx context.Context, areaID string) (*model.AreaSignals, error)
}

// Handler holds HTTP handlers for the pricing service.
type Handler struct {
	svc PricingOrchestrator
}

// New creates a new Handler.
func New(svc PricingOrchestrator) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all pricing service routes on the Gin engine.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	v1 := r.Group("/api/v1")
	v1.POST("/delivery-fee", h.DeliveryFee)
	v1.GET("/locked-price", h.LockedPrice)

	admin := r.Group("/admin")
	admin.GET("/signals/:area_id", h.AreaSignals)
}

// DeliveryFee handles POST /api/v1/delivery-fee
func (h *Handler) DeliveryFee(c *gin.Context) {
	var req model.FeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.Calculate(c.Request.Context(), &req)
	if err != nil {
		kit.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, kit.Response{Data: resp})
}

// LockedPrice handles GET /api/v1/locked-price?order_id=X
func (h *Handler) LockedPrice(c *gin.Context) {
	orderID := c.Query("order_id")
	if orderID == "" {
		kit.BadRequest(c, "order_id is required")
		return
	}

	fee, err := h.svc.GetLockedPrice(c.Request.Context(), orderID)
	if err != nil {
		kit.NotFound(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, kit.Response{
		Data: gin.H{
			"order_id":     orderID,
			"delivery_fee": fee,
		},
	})
}

// AreaSignals handles GET /admin/signals/:area_id
func (h *Handler) AreaSignals(c *gin.Context) {
	areaID := c.Param("area_id")
	if areaID == "" {
		kit.BadRequest(c, "area_id is required")
		return
	}

	signals, err := h.svc.GetSignals(c.Request.Context(), areaID)
	if err != nil {
		kit.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, kit.Response{Data: signals})
}
