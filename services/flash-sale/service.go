package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Checkout pipeline:
// 1. Idempotency guard
// 2. HMAC attestation
// 3. Sliding window rate limit per device_fp
// 4. Atomic bucket stock decrement (Lua)
// 5. On success -> create reservation. On failure -> waiting room.
func (s *Service) Checkout(ctx context.Context, req CheckoutRequest) (*CheckoutResponse, error) {
	// Step 1: Idempotency
	conflict, cached, err := s.store.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency: %w", err)
	}
	if conflict {
		var resp CheckoutResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			resp.Status = StatusIdempotencyConflict
			return &resp, nil
		}
		return &CheckoutResponse{Status: StatusIdempotencyConflict}, nil
	}

	// Step 2: Attestation
	if !s.store.VerifyAttestation(req.DeviceFP, req.ExpiresAt, req.Attestation) {
		resp := &CheckoutResponse{Status: StatusInvalidAttestation}
		s.cacheResult(ctx, req.IdempotencyKey, resp)
		return resp, nil
	}

	// Step 3: Rate limit
	allowed, err := s.store.CheckRateLimit(ctx, req.DeviceFP)
	if err != nil || !allowed {
		resp := &CheckoutResponse{Status: StatusRateLimited}
		s.cacheResult(ctx, req.IdempotencyKey, resp)
		return resp, nil
	}

	// Step 4: Atomic stock reserve
	code, _, err := s.store.ReserveStock(ctx, req.ProductID, req.DeviceFP, req.Quantity, defaultBucketCount)
	if err != nil {
		return nil, fmt.Errorf("reserve stock: %w", err)
	}

	if code >= 0 {
		// Success -> create reservation
		reservationID := generateID("RES")
		s.store.CreateReservation(ctx, req.ProductID, reservationID, req.UserID, req.DeviceFP, req.Quantity, code)

		resp := &CheckoutResponse{
			OrderID:       generateID("ORD"),
			ReservationID: reservationID,
			Status:        StatusCompleted,
		}
		s.cacheResult(ctx, req.IdempotencyKey, resp)
		return resp, nil
	}

	// Stock failed -> waiting room
	pos, err := s.store.JoinWaitingRoom(ctx, req.ProductID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("waiting room: %w", err)
	}

	resp := &CheckoutResponse{Position: pos + 1, Status: StatusQueued}
	s.cacheResult(ctx, req.IdempotencyKey, resp)
	return resp, nil
}

func (s *Service) cacheResult(ctx context.Context, idemKey string, resp *CheckoutResponse) {
	b, _ := json.Marshal(resp)
	s.store.CacheIdempotencyResult(ctx, idemKey, string(b))
}

func (s *Service) ReleaseReservation(ctx context.Context, reservationID string) error {
	return s.store.ReleaseReservation(ctx, reservationID)
}

func (s *Service) QueueStatus(ctx context.Context, productID, userID string) (*QueueStatusResponse, error) {
	pos, err := s.store.QueuePosition(ctx, productID, userID)
	if err != nil {
		return nil, err
	}
	resp := &QueueStatusResponse{ProductID: productID, UserID: userID}
	if pos < 0 {
		resp.Status = "sold_out"
	} else {
		resp.Status = StatusQueued
		resp.Position = pos + 1
	}
	return resp, nil
}

func (s *Service) GenerateToken(deviceFP string) (*TokenResponse, error) {
	token, _ := s.store.GenerateToken(deviceFP)
	return &TokenResponse{Token: token, ExpiresIn: 30}, nil
}
