package main

import (
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// POST /flash-sale/checkout
func (h *Handler) Checkout(c *gin.Context) {
	var req CheckoutRequest
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
	case StatusInvalidAttestation:
		c.JSON(http.StatusUnauthorized, resp)
	case StatusRateLimited:
		c.JSON(http.StatusTooManyRequests, resp)
	case StatusIdempotencyConflict:
		c.JSON(http.StatusConflict, resp)
	case StatusSlotFull:
		c.JSON(http.StatusServiceUnavailable, resp)
	default:
		kit.OK(c, resp)
	}
}

// POST /flash-sale/release
func (h *Handler) Release(c *gin.Context) {
	var req ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "reservation_id required")
		return
	}
	if err := h.svc.ReleaseReservation(c.Request.Context(), req.ReservationID, req.ProductID, req.UserID); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}
	kit.OK(c, gin.H{"released": true})
}

// GET /flash-sale/queue-status?product_id=X&user_id=Y
func (h *Handler) QueueStatus(c *gin.Context) {
	resp, err := h.svc.QueueStatus(c.Request.Context(), c.Query("product_id"), c.Query("user_id"))
	if err != nil {
		kit.InternalError(c, "queue status failed")
		return
	}
	kit.OK(c, resp)
}

// GET /flash-sale/token?device_fp=X
func (h *Handler) Token(c *gin.Context) {
	deviceFP := c.Query("device_fp")
	if deviceFP == "" {
		kit.BadRequest(c, "device_fp required")
		return
	}
	resp, _ := h.svc.GenerateToken(deviceFP)
	kit.OK(c, resp)
}

// GET /flash-sale/slot-status?product_id=X
func (h *Handler) SlotStatus(c *gin.Context) {
	productID := c.Query("product_id")
	if productID == "" {
		kit.BadRequest(c, "product_id required")
		return
	}
	resp, err := h.svc.SlotStatus(c.Request.Context(), productID)
	if err != nil {
		kit.InternalError(c, "slot status failed")
		return
	}
	kit.OK(c, resp)
}

func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/flash-sale/checkout", h.Checkout)
	r.POST("/flash-sale/release", h.Release)
	r.GET("/flash-sale/queue-status", h.QueueStatus)
	r.GET("/flash-sale/token", h.Token)
	r.GET("/flash-sale/slot-status", h.SlotStatus)
}
