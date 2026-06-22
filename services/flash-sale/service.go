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
// 4. Slot Pool semaphore — batasi sesi konkuren sesuai kapasitas DB
// 5. Atomic bucket stock decrement (Lua)
// 6. On success -> create reservation. On failure -> release slot + waiting room.
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

	// Step 4: Slot Pool — batasi sesi konkuren sesuai kapasitas DB.
	// Rate limiter nahan banjir per detik, tapi gak ngelindungin DB dari
	// 1000 sesi yang jalan barengan. Slot Pool yang ngelimit itu.
	sessionID := req.ProductID + ":" + req.UserID
	acquired, _, err := s.store.AcquireSlot(ctx, req.ProductID, sessionID, defaultMaxSlots, defaultSlotTTL)
	if err != nil {
		return nil, fmt.Errorf("slot pool: %w", err)
	}
	if !acquired {
		resp := &CheckoutResponse{Status: StatusSlotFull}
		s.cacheResult(ctx, req.IdempotencyKey, resp)
		return resp, nil
	}

	// Step 5: Atomic stock reserve
	code, _, err := s.store.ReserveStock(ctx, req.ProductID, req.DeviceFP, req.Quantity, defaultBucketCount)
	if err != nil {
		// Gagal reserve → lepas slot. Jangan sampai slot nempel padahal gak jadi.
		s.store.ReleaseSlot(ctx, req.ProductID, sessionID)
		return nil, fmt.Errorf("reserve stock: %w", err)
	}

	if code >= 0 {
		// Success → reservation created. Slot tetap dipegang (TTL 15 menit)
		// buat ngelindungin DB selama transaksi (termasuk payment).
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

	// Stock sold out → lepas slot, masuk ruang tunggu.
	// Slot gak relevan lagi karena user gak akan nyentuh DB sampe stok balik.
	s.store.ReleaseSlot(ctx, req.ProductID, sessionID)

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

func (s *Service) ReleaseReservation(ctx context.Context, reservationID, productID, userID string) error {
	if err := s.store.ReleaseReservation(ctx, reservationID); err != nil {
		return err
	}
	// Lepas slot pool — stok udah balik, slot juga harus balik.
	// Abaikan error: slot mungkin udah expired TTL.
	_ = s.store.ReleaseSlot(ctx, productID, productID+":"+userID)
	return nil
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

func (s *Service) SlotStatus(ctx context.Context, productID string) (*SlotPoolResponse, error) {
	avail, err := s.store.SlotsAvailable(ctx, productID, defaultMaxSlots)
	if err != nil {
		return nil, err
	}
	return &SlotPoolResponse{
		Available:   avail,
		Max:         defaultMaxSlots,
		HasCapacity: avail > 0,
	}, nil
}
