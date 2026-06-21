package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit/circuitbreaker"
	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit/middleware"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/repository"
	"github.com/gin-gonic/gin"
)

type FlashSaleService interface {
	Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error)
	QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error)
	ReleaseReservation(ctx context.Context, reservationID string) error
	ConfirmReservation(ctx context.Context, reservationID string) error
	DryRun(ctx context.Context, req model.CheckoutRequest) (*model.DryRunResponse, error)
}

type FlashSaleHandler struct {
	svc        FlashSaleService
	repo       *repository.FlashSaleRepo
	hmacSecret []byte
	cb         *circuitbreaker.CircuitBreaker
}

func New(svc FlashSaleService, repo *repository.FlashSaleRepo, hmacSecret string, cb *circuitbreaker.CircuitBreaker) *FlashSaleHandler {
	return &FlashSaleHandler{svc: svc, repo: repo, hmacSecret: []byte(hmacSecret), cb: cb}
}

// GET /flash-sale/token?device_fp=X
func (h *FlashSaleHandler) Token(c *gin.Context) {
	deviceFP := c.Query("device_fp")
	if deviceFP == "" {
		kit.BadRequest(c, "device_fp query param is required")
		return
	}
	// Generate HMAC token (server-side, shared secret never in client)
	mac := hmac.New(sha256.New, h.hmacSecret)
	payload := fmt.Sprintf("%s:%d", deviceFP, time.Now().Unix()+30)
	mac.Write([]byte(payload))
	token := hex.EncodeToString(mac.Sum(nil))
	kit.OK(c, gin.H{"token": token, "expires_in": 30})
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
		c.JSON(http.StatusUnauthorized, resp)
	case model.StatusRateLimited:
		c.JSON(http.StatusTooManyRequests, resp)
	case model.StatusIdempotencyConflict:
		c.JSON(http.StatusConflict, resp)
	default:
		kit.OK(c, resp)
	}
}

// POST /flash-sale/release — Gap 6: compensate failed checkout
func (h *FlashSaleHandler) Release(c *gin.Context) {
	var req model.ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "reservation_id is required")
		return
	}
	if err := h.svc.ReleaseReservation(c.Request.Context(), req.ReservationID); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}
	kit.OK(c, gin.H{"released": true})
}

// POST /flash-sale/confirm — finalize successful checkout
func (h *FlashSaleHandler) Confirm(c *gin.Context) {
	var req model.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "reservation_id is required")
		return
	}
	if err := h.svc.ConfirmReservation(c.Request.Context(), req.ReservationID); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}
	kit.OK(c, gin.H{"confirmed": true})
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

// GET /flash-sale/queue-stream?product_id=X&user_id=Y — Gap 7: SSE push alternative to polling
func (h *FlashSaleHandler) QueueStream(c *gin.Context) {
	productID := c.Query("product_id")
	userID := c.Query("user_id")
	if productID == "" || userID == "" {
		kit.BadRequest(c, "product_id and user_id are required")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial position
	pos, err := h.svc.QueueStatus(c.Request.Context(), productID, userID)
	if err == nil {
		data, _ := json.Marshal(pos)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}

	// Subscribe to Redis pub/sub for position updates
	pubsub := h.repo.SubscribeQueue(c.Request.Context(), productID, userID)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", msg.Payload)
			c.Writer.Flush()
		}
	}
}

// POST /flash-sale/dry-run
func (h *FlashSaleHandler) DryRun(c *gin.Context) {
	var req model.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}
	resp, err := h.svc.DryRun(c.Request.Context(), req)
	if err != nil {
		kit.InternalError(c, "dry-run failed")
		return
	}
	middleware.GlobalMetrics.Inc("flash_dry_run_total")
	if resp.WouldSucceed {
		middleware.GlobalMetrics.Inc("flash_dry_run_success")
	}
	kit.OK(c, resp)
}

// GET /flash-sale/metrics
func (h *FlashSaleHandler) Metrics(c *gin.Context) {
	snapshot := middleware.GlobalMetrics.Snapshot()
	kit.OK(c, snapshot)
}

// GET /flash-sale/health
func (h *FlashSaleHandler) Health(c *gin.Context) {
	state := "closed"
	if h.cb != nil && h.cb.State() == circuitbreaker.Open {
		state = "open"
	}
	failures, threshold := 0, 0
	if h.cb != nil {
		failures, threshold = h.cb.Counts()
	}
	kit.OK(c, gin.H{
		"status":         "ok",
		"circuit_breaker": state,
		"failures":       failures,
		"threshold":      threshold,
	})
}

func (h *FlashSaleHandler) Register(r *gin.RouterGroup) {
	r.POST("/flash-sale/checkout", h.Checkout)
	r.POST("/flash-sale/release", h.Release)
	r.POST("/flash-sale/confirm", h.Confirm)
	r.GET("/flash-sale/queue-status", h.QueueStatus)
	r.GET("/flash-sale/queue-stream", h.QueueStream)
	r.GET("/flash-sale/token", middleware.CDNCache(middleware.DefaultCacheConfig()), h.Token)
	r.POST("/flash-sale/dry-run", h.DryRun)
	r.GET("/flash-sale/metrics", h.Metrics)
	r.GET("/flash-sale/health", h.Health)
}
