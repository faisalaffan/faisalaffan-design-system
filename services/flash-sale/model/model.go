package model

import "time"

const (
	StatusQueued             = "queued"
	StatusCompleted          = "completed"
	StatusSoldOut            = "sold_out"
	StatusRateLimited        = "rate_limited"
	StatusInvalidAttestation = "invalid_attestation"

	DefaultBucketCount     = 10
	DefaultRateLimitWindow = time.Second
	DefaultRateLimitBurst  = 20 // limit * 2
	AttestationMaxSkew     = 30 * time.Second
)

type FlashSaleProduct struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	BucketCount int     `json:"bucket_count"`
}

type CheckoutRequest struct {
	ProductID   string `json:"product_id" binding:"required"`
	UserID      string `json:"user_id" binding:"required"`
	DeviceFP    string `json:"device_fp" binding:"required"`
	Attestation string `json:"attestation" binding:"required"`
	ExpiresAt   int64  `json:"expires_at" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,min=1"`
}

type CheckoutResponse struct {
	OrderID  string `json:"order_id,omitempty"`
	Position int    `json:"position,omitempty"`
	Status   string `json:"status"`
}

type QueueStatusResponse struct {
	Position  int    `json:"position"`
	Status    string `json:"status"`
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
}
