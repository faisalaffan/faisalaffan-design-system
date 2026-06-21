package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/repository"
	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/service"
	"github.com/gin-gonic/gin"
)

// Handler wires HTTP handlers to the checkout service.
type Handler struct {
	svc       *service.CheckoutService
	redisRepo *repository.RedisClient
	hmacKey   []byte
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(svc *service.CheckoutService, redisRepo *repository.RedisClient) *Handler {
	key := os.Getenv("WEBHOOK_SECRET")
	if key == "" {
		key = "dev-secret-do-not-use-in-production"
	}
	return &Handler{
		svc:       svc,
		redisRepo: redisRepo,
		hmacKey:   []byte(key),
	}
}

// RegisterRoutes mounts all routes on the given engine group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/checkout", h.IdempotencyMiddleware(), h.Checkout)
	rg.POST("/webhook/payment", h.WebhookPayment)
	rg.GET("/orders/:id", h.GetOrder)
}

// ─── POST /checkout ──────────────────────────────────────────────────────────

// Checkout handles the checkout request.
func (h *Handler) Checkout(c *gin.Context) {
	var req model.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}

	ctx := &model.SagaContext{
		UserID:   req.UserID,
		Items:    req.Items,
		Currency: req.Payment.Currency,
	}

	result, err := h.svc.NewOrder(ctx)
	if err != nil {
		log.Printf("handler: checkout error: %v", err)
		kit.InternalError(c, "checkout failed")
		return
	}

	if !result.Success {
		kit.BadRequest(c, "checkout failed at step "+string(*result.FailedAt))
		return
	}

	// Cache the idempotency result.
	if key, exists := c.Get("idempotency_key"); exists {
		h.redisRepo.StoreResult(c.Request.Context(), key.(string), result)
	}

	kit.Created(c, model.CheckoutResponse{
		OrderID:     result.OrderID,
		Status:      result.Order.Status,
		Transaction: "txn_" + result.OrderID,
	})
}

// ─── POST /webhook/payment ───────────────────────────────────────────────────

// WebhookPayment handles incoming payment webhooks with HMAC verification.
func (h *Handler) WebhookPayment(c *gin.Context) {
	// Read raw body for signature verification.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		kit.BadRequest(c, "cannot read body")
		return
	}
	// Restore body for potential re-reading.
	c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	// HMAC-SHA256 verification.
	sig := c.GetHeader("X-Signature")
	if sig == "" {
		kit.BadRequest(c, "missing X-Signature header")
		return
	}

	if !h.verifyHMAC(rawBody, sig) {
		kit.BadRequest(c, "invalid signature")
		return
	}

	var payload model.WebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		kit.BadRequest(c, "invalid webhook payload")
		return
	}

	if payload.TransactionID == "" || payload.OrderID == "" {
		kit.BadRequest(c, "transaction_id and order_id are required")
		return
	}

	// Webhook idempotency: ensure we haven't processed this transaction.
	ctx := context.Background()
	if h.redisRepo.IsWebhookProcessed(ctx, payload.TransactionID) {
		log.Printf("webhook: duplicate txID=%s, skipping", payload.TransactionID)
		kit.OK(c, gin.H{"status": "already_processed"})
		return
	}
	if !h.redisRepo.MarkWebhookProcessed(ctx, payload.TransactionID) {
		log.Printf("webhook: txID=%s already being processed by another consumer", payload.TransactionID)
		kit.OK(c, gin.H{"status": "already_processing"})
		return
	}

	if payload.Status == "succeeded" {
		if err := h.svc.ConfirmPayment(payload.OrderID); err != nil {
			log.Printf("webhook: confirm payment error: %v", err)
			kit.InternalError(c, "payment confirmation failed")
			return
		}
	} else {
		log.Printf("webhook: payment %s status=%s", payload.TransactionID, payload.Status)
	}

	kit.OK(c, gin.H{"status": "ok"})
}

// ─── GET /orders/:id ─────────────────────────────────────────────────────────

// GetOrder retrieves an order by ID.
func (h *Handler) GetOrder(c *gin.Context) {
	id := c.Param("id")
	order := h.svc.GetOrder(id)
	if order == nil {
		kit.NotFound(c, "order not found")
		return
	}
	kit.OK(c, order)
}

// ─── Idempotency Middleware ──────────────────────────────────────────────────

// IdempotencyMiddleware checks the Idempotency-Key header.
// On duplicate keys, it returns the cached result.
// On first use, it acquires a Redis lock and proceeds.
func (h *Handler) IdempotencyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			kit.BadRequest(c, "Idempotency-Key header is required")
			c.Abort()
			return
		}

		ctx := c.Request.Context()

		// Check if we have a cached result for this key.
		if cached := h.redisRepo.GetResult(ctx, key); cached != nil {
			log.Printf("idempotency: returning cached result for key=%s", key)
			kit.OK(c, cached)
			c.Abort()
			return
		}

		// Acquire lock to prevent concurrent duplicate requests.
		if !h.redisRepo.TryLock(ctx, key) {
			// If we can't acquire the lock, another request is being processed.
			c.JSON(http.StatusConflict, gin.H{"error": "request already in progress"})
			c.Abort()
			return
		}

		c.Set("idempotency_key", key)
		c.Next()
	}
}

// ─── HMAC Verification ───────────────────────────────────────────────────────

func (h *Handler) verifyHMAC(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, h.hmacKey)
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
