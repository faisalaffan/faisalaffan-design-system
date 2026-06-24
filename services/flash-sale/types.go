package main

import "time"

const (
	StatusCompleted           = "completed"
	StatusQueued              = "queued"
	StatusRateLimited         = "rate_limited"
	StatusInvalidAttestation  = "invalid_attestation"
	StatusIdempotencyConflict = "idempotency_conflict"
	StatusSlotFull            = "slot_full"
	StatusLotteryEntered      = "lottery_entered"
	StatusLotteryWinner       = "lottery_winner"
	StatusLotteryLoser        = "lottery_loser"

	defaultBucketCount       = 10
	defaultRateLimitWindow   = time.Second
	defaultRateLimitBurst    = 20
	attestationMaxSkew       = 30 * time.Second
	reservationTTL           = 5 * time.Minute
	waitingRoomTTL           = 10 * time.Minute
	defaultMaxSlots          = 1000
	defaultSlotTTL           = 15 * time.Minute
	defaultLotteryWindow     = 2 * time.Hour
	defaultLotteryCheckoutTTL = 10 * time.Minute
)

type CheckoutRequest struct {
	ProductID      string `json:"product_id" binding:"required"`
	UserID         string `json:"user_id" binding:"required"`
	DeviceFP       string `json:"device_fp" binding:"required"`
	Attestation    string `json:"attestation" binding:"required"`
	ExpiresAt      int64  `json:"expires_at" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
	LotteryToken   string `json:"lottery_token,omitempty"`
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

type ReleaseRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
	ProductID     string `json:"product_id" binding:"required"`
	UserID        string `json:"user_id" binding:"required"`
}

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

type SlotPoolResponse struct {
	Available   int  `json:"available"`
	Max         int  `json:"max"`
	HasCapacity bool `json:"has_capacity"`
}

type LotteryEnterRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	UserID    string `json:"user_id" binding:"required"`
	DeviceFP  string `json:"device_fp" binding:"required"`
}

type LotteryDrawRequest struct {
	ProductID   string `json:"product_id" binding:"required"`
	WinnerCount int    `json:"winner_count" binding:"required,min=1"`
}

type LotteryResultResponse struct {
	Status        string  `json:"status"`
	ProductID     string  `json:"product_id"`
	UserID        string  `json:"user_id"`
	EntryCount    int     `json:"entry_count,omitempty"`
	WinnerCount   int     `json:"winner_count,omitempty"`
	ReservationID string  `json:"reservation_id,omitempty"`
	LotteryToken  string  `json:"lottery_token,omitempty"`
	CheckoutURL   string  `json:"checkout_url,omitempty"`
	ExpiresAt     int64   `json:"expires_at,omitempty"`
	OddsPercent   float64 `json:"odds_percent,omitempty"`
}
