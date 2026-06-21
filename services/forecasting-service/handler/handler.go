package handler

import (
	"context"
	"net/http"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
	"github.com/gin-gonic/gin"
)

// ForecastingOrchestrator defines the interface for the forecasting service.
type ForecastingOrchestrator interface {
	RunDailyForecast(ctx context.Context, hubIDs []string) ([]model.ForecastResponse, error)
	GetForecast(sku, hubID string) (model.ForecastResult, bool)
	GetReplenishment(sku, hubID string) (model.ReplenishmentResult, bool)
	GetAccuracyStats(sku, hubID string) model.SKUForecastStats
	GetDegradedSKUs() []string
	SeedExternalFeatures(sku, hubID string, feat model.ExternalFeature)
}

// Handler holds HTTP handlers for the forecasting service.
type Handler struct {
	svc ForecastingOrchestrator
}

// New creates a new Handler.
func New(svc ForecastingOrchestrator) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all forecasting service routes.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	v1 := r.Group("/api/v1")
	forecast := v1.Group("/forecast")
	{
		forecast.POST("/run", h.RunForecast)
		forecast.GET("/:sku/:hub_id", h.GetForecast)
		forecast.GET("/accuracy/:sku/:hub_id", h.GetForecastAccuracy)
	}

	admin := r.Group("/admin")
	{
		admin.POST("/features", h.SeedFeatures)
		admin.GET("/degraded", h.ListDegraded)
	}
}

// RunForecast handles POST /api/v1/forecast/run
func (h *Handler) RunForecast(c *gin.Context) {
	var req struct {
		HubIDs []string `json:"hub_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "hub_ids is required")
		return
	}

	results, err := h.svc.RunDailyForecast(c.Request.Context(), req.HubIDs)
	if err != nil {
		// Return partial results on error
		c.JSON(http.StatusOK, gin.H{
			"data":  results,
			"error": err.Error(),
		})
		return
	}

	kit.OK(c, results)
}

// GetForecast handles GET /api/v1/forecast/:sku/:hub_id
func (h *Handler) GetForecast(c *gin.Context) {
	sku := c.Param("sku")
	hubID := c.Param("hub_id")

	forecast, ok := h.svc.GetForecast(sku, hubID)
	if !ok {
		kit.NotFound(c, "forecast not found for this SKU-hub pair")
		return
	}

	replen, _ := h.svc.GetReplenishment(sku, hubID)

	kit.OK(c, gin.H{
		"forecast":      forecast,
		"replenishment": replen,
	})
}

// GetForecastAccuracy handles GET /api/v1/forecast/accuracy/:sku/:hub_id
func (h *Handler) GetForecastAccuracy(c *gin.Context) {
	sku := c.Param("sku")
	hubID := c.Param("hub_id")

	stats := h.svc.GetAccuracyStats(sku, hubID)
	kit.OK(c, stats)
}

// SeedFeatures handles POST /admin/features
func (h *Handler) SeedFeatures(c *gin.Context) {
	var req struct {
		SKU   string `json:"sku" binding:"required"`
		HubID string `json:"hub_id" binding:"required"`
		model.ExternalFeature
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}

	h.svc.SeedExternalFeatures(req.SKU, req.HubID, req.ExternalFeature)
	kit.OK(c, gin.H{"status": "seeded"})
}

// ListDegraded handles GET /admin/degraded
func (h *Handler) ListDegraded(c *gin.Context) {
	degraded := h.svc.GetDegradedSKUs()
	kit.OK(c, gin.H{"degraded_skus": degraded})
}
