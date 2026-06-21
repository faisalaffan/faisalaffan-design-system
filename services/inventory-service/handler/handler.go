package handler

import (
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/service"
	"github.com/gin-gonic/gin"
)

type InventoryHandler struct {
	svc *service.InventoryService
}

func NewInventoryHandler(svc *service.InventoryService) *InventoryHandler {
	return &InventoryHandler{svc: svc}
}

func (h *InventoryHandler) GetStock(c *gin.Context) {
	hubID := c.Param("hub_id")
	sku := c.Param("sku")
	info, err := h.svc.GetStock(c.Request.Context(), hubID, sku)
	if err != nil {
		kit.InternalError(c, "failed to get stock")
		return
	}
	kit.OK(c, info)
}

func (h *InventoryHandler) Reserve(c *gin.Context) {
	var req model.ReserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request: hub_id, cart_id, user_id, items required")
		return
	}
	resp, err := h.svc.Reserve(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusConflict, kit.Response{Error: err.Error()})
		return
	}
	kit.Created(c, resp)
}

func (h *InventoryHandler) Release(c *gin.Context) {
	var req model.ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "reservation_id required")
		return
	}
	if err := h.svc.Release(c.Request.Context(), req.ReservationID, c.Query("hub_id"), c.Query("sku")); err != nil {
		c.JSON(http.StatusConflict, kit.Response{Error: err.Error()})
		return
	}
	kit.OK(c, gin.H{"released": true})
}

func (h *InventoryHandler) Confirm(c *gin.Context) {
	var req model.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "reservation_id required")
		return
	}
	if err := h.svc.Confirm(c.Request.Context(), req.ReservationID, c.Query("hub_id"), c.Query("sku")); err != nil {
		c.JSON(http.StatusConflict, kit.Response{Error: err.Error()})
		return
	}
	kit.OK(c, gin.H{"confirmed": true})
}

func (h *InventoryHandler) SetStock(c *gin.Context) {
	var req struct {
		HubID    string `json:"hub_id" binding:"required"`
		SKU      string `json:"sku" binding:"required"`
		Quantity int    `json:"quantity" binding:"required,min=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "hub_id, sku, quantity required")
		return
	}
	if err := h.svc.SetStock(c.Request.Context(), req.HubID, req.SKU, req.Quantity); err != nil {
		kit.InternalError(c, "failed to set stock")
		return
	}
	kit.OK(c, gin.H{"set": true})
}

func (h *InventoryHandler) Register(r *gin.RouterGroup) {
	r.GET("/stock/:hub_id/:sku", h.GetStock)
	r.POST("/reserve", h.Reserve)
	r.POST("/release", h.Release)
	r.POST("/confirm", h.Confirm)
	r.POST("/admin/stock", h.SetStock)
}
