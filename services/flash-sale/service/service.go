package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/event"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/flash-sale/repository"
)

type FlashSaleService struct {
	repo        *repository.FlashSaleRepo
	hmacSecret  []byte
	rateWindow  time.Duration
	rateBurst   int
	bucketCount int
	resTTL      time.Duration
	eventPub    event.Publisher
}

func New(repo *repository.FlashSaleRepo, hmacSecret string, eventPub event.Publisher) *FlashSaleService {
	return &FlashSaleService{
		repo:        repo,
		hmacSecret:  []byte(hmacSecret),
		eventPub:    eventPub,
		rateWindow:  model.DefaultRateLimitWindow,
		rateBurst:   model.DefaultRateLimitBurst,
		bucketCount: model.DefaultBucketCount,
		resTTL:      model.DefaultReservationTTL,
	}
}

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
		return fmt.Sprintf("ORD-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ORD-%x-%x", time.Now().UnixMilli(), b)
}

func generateReservationID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("RES-%x", b)
}

// Checkout pipeline:
//   1. Idempotency guard (SetNX lock + cached result) — prevents double-submit
//   2. HMAC attestation verification (local, shared secret) — anti-bot
//   3. Sliding-window rate limit per device fingerprint — not per IP
//   4. Atomic bucket stock decrement + reservation with TTL — rollback on failure
//   5. On stock success → completed; on failure → waiting room
func (s *FlashSaleService) Checkout(ctx context.Context, req model.CheckoutRequest) (*model.CheckoutResponse, error) {
	// Gap 2 fix: idempotency guard
	conflict, cached, err := s.repo.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency: %w", err)
	}
	if conflict {
		if cached != "" {
			var resp model.CheckoutResponse
			if json.Unmarshal([]byte(cached), &resp) == nil {
				resp.Status = model.StatusIdempotencyConflict
				return &resp, nil
			}
		}
		return &model.CheckoutResponse{Status: model.StatusIdempotencyConflict}, nil
	}

	// Gap 3: local HMAC verification — correct trade-off for flash sale latency
	if !s.verifyAttestation(req.DeviceFP, req.ExpiresAt, req.Attestation) {
		s.cacheResult(ctx, req.IdempotencyKey, model.CheckoutResponse{Status: model.StatusInvalidAttestation})
		return &model.CheckoutResponse{Status: model.StatusInvalidAttestation}, nil
	}

	// Gap 4: rate limit per device fingerprint (FNV hash) — not IP-based
	allowed, err := s.repo.CheckRateLimit(ctx, req.DeviceFP, s.rateWindow, s.rateBurst)
	if err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	if !allowed {
		s.cacheResult(ctx, req.IdempotencyKey, model.CheckoutResponse{Status: model.StatusRateLimited})
		return &model.CheckoutResponse{Status: model.StatusRateLimited}, nil
	}

	// Gap 1 + 6: atomic reserve with TTL → creates reservation for rollback
	reservationID := generateReservationID()
	bucketIdx, _, _, err := s.repo.BucketDecrement(
		ctx, req.ProductID, reservationID, req.Quantity, s.bucketCount,
		req.DeviceFP, req.UserID, s.resTTL,
	)
	if err != nil {
		// Gap 6: if bucket decrement fails after partial work, Lua script is atomic — no partial state
		return nil, fmt.Errorf("bucket decrement: %w", err)
	}

	if bucketIdx >= 0 {
		resp := &model.CheckoutResponse{
			OrderID:       generateOrderID(),
			ReservationID: reservationID,
			Status:        model.StatusCompleted,
		}
		// Gap 1: reservation created with TTL in Lua. Reaper will auto-release if expired.
		s.cacheResult(ctx, req.IdempotencyKey, *resp)

		// Publish order created event (async, non-blocking)
		evt := event.OrderCreatedEvent{
			OrderID:       resp.OrderID,
			UserID:        req.UserID,
			ProductID:     req.ProductID,
			Quantity:      req.Quantity,
			ReservationID: reservationID,
			DeviceFP:      req.DeviceFP,
		}
		if err := s.eventPub.PublishOrderCreated(ctx, evt); err != nil {
			log.Printf("warn: event publish failed for order %s: %v", resp.OrderID, err)
		}

		return resp, nil
	}

	// Stock failed → join waiting room
	pos, err := s.repo.JoinWaitingRoom(ctx, req.ProductID, req.UserID)
	if err != nil {
		// Gap 6: compensation — release reservation if waiting room join fails
		if reservationID != "" {
			s.repo.ReleaseReservation(ctx, reservationID)
		}
		return nil, fmt.Errorf("join waiting room: %w", err)
	}

	// Gap 5: waiting room has TTL via periodic cleanup
	resp := &model.CheckoutResponse{Position: pos + 1, Status: model.StatusQueued}
	s.cacheResult(ctx, req.IdempotencyKey, *resp)
	return resp, nil
}

func (s *FlashSaleService) cacheResult(ctx context.Context, idemKey string, resp model.CheckoutResponse) {
	b, _ := json.Marshal(resp)
	if err := s.repo.StoreIdempotencyResult(ctx, idemKey, string(b)); err != nil {
		log.Printf("warn: failed to cache idempotency result: %v", err)
	}
}

// ReleaseReservation compensates a failed checkout — returns stock.
func (s *FlashSaleService) ReleaseReservation(ctx context.Context, reservationID string) error {
	return s.repo.ReleaseReservation(ctx, reservationID)
}

// ConfirmReservation finalizes a successful checkout.
func (s *FlashSaleService) ConfirmReservation(ctx context.Context, reservationID string) error {
	return s.repo.ConfirmReservation(ctx, reservationID)
}

func (s *FlashSaleService) QueueStatus(ctx context.Context, productID, userID string) (*model.QueueStatusResponse, error) {
	pos, err := s.repo.QueuePosition(ctx, productID, userID)
	if err != nil {
		return nil, fmt.Errorf("queue position: %w", err)
	}
	resp := &model.QueueStatusResponse{ProductID: productID, UserID: userID}
	if pos < 0 {
		resp.Status = model.StatusSoldOut
	} else {
		resp.Status = model.StatusQueued
		resp.Position = pos + 1
	}
	return resp, nil
}

// StartBackgroundJobs launches reaper + waiting room cleanup goroutines.
func (s *FlashSaleService) StartBackgroundJobs(ctx context.Context, productID string) {
	// Gap 1: reaper releases expired reservations
	go func() {
		ticker := time.NewTicker(model.ReaperInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.repo.RunReaper(ctx, productID, model.ReaperGracePeriod)
				if err != nil {
					log.Printf("reaper error: %v", err)
				} else if n > 0 {
					log.Printf("reaper: released %d expired reservations", n)
				}
			}
		}
	}()

	// Gap 5: waiting room cleanup removes stale entries
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, _ := s.repo.CleanWaitingRoom(ctx, productID, model.WaitingRoomTTL)
				if n > 0 {
					log.Printf("waiting room: removed %d stale entries", n)
				}
			}
		}
	}()

	// Gap 5: admission consumer processes waiting room → stock reservation
	go func() {
		ticker := time.NewTicker(model.AdmissionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				users, err := s.repo.AdmitNext(ctx, productID, model.AdmissionBatchSize)
				if err != nil || len(users) == 0 {
					continue
				}
				log.Printf("admission: admitted %d users from waiting room", len(users))
				for _, userID := range users {
					s.repo.PublishQueueEvent(ctx, productID, userID, model.QueueEvent{
						Position: 0, Status: "admitted",
					})
				}
			}
		}
	}()
}
