package model

import "time"

const (
	StatusQueued             = "queued"
	StatusCompleted          = "completed"
	StatusSoldOut            = "sold_out"
	StatusRateLimited        = "rate_limited"
	StatusInvalidAttestation = "invalid_attestation"
	StatusIdempotencyConflict = "idempotency_conflict"
	StatusReserved           = "reserved"
	StatusReleased           = "released"
	StatusExpired            = "expired"

	DefaultBucketCount       = 10
	DefaultRateLimitWindow   = time.Second
	DefaultRateLimitBurst    = 20
	AttestationMaxSkew       = 30 * time.Second
	DefaultReservationTTL    = 5 * time.Minute
	WaitingRoomTTL           = 10 * time.Minute
	ReaperInterval           = 10 * time.Second
	ReaperGracePeriod        = 15 * time.Second
	AdmissionBatchSize       = 10
	AdmissionInterval        = 500 * time.Millisecond
)

type FlashSaleProduct struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	BucketCount int     `json:"bucket_count"`
}

type CheckoutRequest struct {
	ProductID      string `json:"product_id" binding:"required"`
	UserID         string `json:"user_id" binding:"required"`
	DeviceFP       string `json:"device_fp" binding:"required"`
	Attestation    string `json:"attestation" binding:"required"`
	ExpiresAt      int64  `json:"expires_at" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

type CheckoutResponse struct {
	OrderID       string `json:"order_id,omitempty"`
	ReservationID string `json:"reservation_id,omitempty"`
	Position      int    `json:"position,omitempty"`
	Status        string `json:"status"`
}

type QueueStatusResponse struct {
	Position  int    `json:"position"`
	Status    string `json:"status"`
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
}

type Reservation struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
	Quantity  int    `json:"quantity"`
	BucketIdx int    `json:"bucket_idx"`
	Status    string `json:"status"` // reserved, confirmed, released, expired
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type ReleaseRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}

type ConfirmRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}

// SSE queue status event
type QueueEvent struct {
	Position int    `json:"position"`
	Status   string `json:"status"`
}
