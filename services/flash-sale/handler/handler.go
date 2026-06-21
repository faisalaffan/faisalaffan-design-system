package handler

import (
	"context"
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/gin-gonic/gin"
)

// FlashSaleService is the business-logic contract consumed by the HTTP handler.
type FlashSaleService interface {
	Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error)
	QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error)
}

type FlashSaleHandler struct {
	svc FlashSaleService
}

func New(svc FlashSaleService) *FlashSaleHandler {
	return &FlashSaleHandler{svc: svc}
}

// POST /flash-sale/checkout
func (h *FlashSaleHandler) Checkout(c *gin.Context) {
	var req model.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.Checkout(c.Request.Context(), req)
	if err != nil {
		kit.InternalError(c, "checkout failed")
		return
	}

	switch resp.Status {
	case model.StatusInvalidAttestation:
		kit.BadRequest(c, "invalid attestation token")
	case model.StatusRateLimited:
		c.JSON(http.StatusTooManyRequests, resp)
	default:
		kit.OK(c, resp)
	}
}

// GET /flash-sale/queue-status?product_id=X&user_id=Y
func (h *FlashSaleHandler) QueueStatus(c *gin.Context) {
	productID := c.Query("product_id")
	userID := c.Query("user_id")
	if productID == "" || userID == "" {
		kit.BadRequest(c, "product_id and user_id are required")
		return
	}

	resp, err := h.svc.QueueStatus(c.Request.Context(), productID, userID)
	if err != nil {
		kit.InternalError(c, "failed to get queue status")
		return
	}

	kit.OK(c, resp)
}

func (h *FlashSaleHandler) Register(r *gin.RouterGroup) {
	r.POST("/flash-sale/checkout", h.Checkout)
	r.GET("/flash-sale/queue-status", h.QueueStatus)
}
