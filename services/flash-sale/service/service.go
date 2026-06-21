package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/repository"
)

type FlashSaleService struct {
	repo        *repository.FlashSaleRepo
	hmacSecret  []byte
	rateWindow  time.Duration
	rateBurst   int
	bucketCount int
}

func New(repo *repository.FlashSaleRepo, hmacSecret string) *FlashSaleService {
	return &FlashSaleService{
		repo:        repo,
		hmacSecret:  []byte(hmacSecret),
		rateWindow:  model.DefaultRateLimitWindow,
		rateBurst:   model.DefaultRateLimitBurst,
		bucketCount: model.DefaultBucketCount,
	}
}

// verifyAttestation checks HMAC-SHA256(secret, deviceFP+":"+expiresAt) matches token,
// and that expiresAt is within 30s of the current wall clock.
func (s *FlashSaleService) verifyAttestation(deviceFP string, expiresAt int64, token string) bool {
	now := time.Now().Unix()
	if delta := now - expiresAt; delta > 30 || delta < -30 {
		return false
	}
	mac := hmac.New(sha256.New, s.hmacSecret)
	mac.Write([]byte(deviceFP + ":" + strconv.FormatInt(expiresAt, 10)))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(token))
}

func generateOrderID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read almost never fails on modern kernels;
		// fall back to nano-precision timestamp if it does.
		return fmt.Sprintf("ORD-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ORD-%x-%x", time.Now().UnixMilli(), b)
}

// Checkout implements the complete flash-sale checkout pipeline:
//   1. Verify HMAC attestation
//   2. Sliding-window rate limit (per device fingerprint)
//   3. Atomic bucket stock decrement (primary bucket via hash, sequential fallthrough)
//   4. On failure → join sorted-set waiting room, return position
func (s *FlashSaleService) Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error) {
	if !s.verifyAttestation(req.DeviceFP, req.ExpiresAt, req.Attestation) {
		return &model.CheckoutResponse{Status: model.StatusInvalidAttestation}, nil
	}

	allowed, err := s.repo.CheckRateLimit(ctx, req.DeviceFP, s.rateWindow, s.rateBurst)
	if err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	if !allowed {
		return &model.CheckoutResponse{Status: model.StatusRateLimited}, nil
	}

	result, err := s.repo.BucketDecrement(ctx, req.ProductID, req.Quantity, s.bucketCount, req.DeviceFP)
	if err != nil {
		return nil, fmt.Errorf("bucket decrement: %w", err)
	}

	if result >= 0 {
		return &model.CheckoutResponse{
			OrderID: generateOrderID(),
			Status:  model.StatusCompleted,
		}, nil
	}

	pos, err := s.repo.JoinWaitingRoom(ctx, req.ProductID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("join waiting room: %w", err)
	}

	return &model.CheckoutResponse{
		Position: pos + 1,
		Status:   model.StatusQueued,
	}, nil
}

// QueueStatus returns the user's current position in the waiting room.
func (s *FlashSaleService) QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error) {
	pos, err := s.repo.QueuePosition(ctx, productID, userID)
	if err != nil {
		return nil, fmt.Errorf("queue position: %w", err)
	}

	resp := &model.QueueStatusResponse{
		ProductID: productID,
		UserID:    userID,
	}
	if pos < 0 {
		resp.Status = model.StatusSoldOut
		resp.Position = 0
	} else {
		resp.Status = model.StatusQueued
		resp.Position = pos + 1
	}
	return resp, nil
}
