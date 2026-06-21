package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
	"github.com/redis/go-redis/v9"
)

const etaCachePrefix = "eta:result:"

// ETAService orchestrates multi-component concurrent ETA calculation.
type ETAService struct {
	config  model.ETAConfig
	picking *PickingEstimator
	travel  *TravelEstimator
	queue   *QueueEstimator
	rdb     *redis.Client
}

// NewETAService creates a new ETAService.
func NewETAService(cfg model.ETAConfig, rdb *redis.Client) *ETAService {
	return &ETAService{
		config:  cfg,
		picking: NewPickingEstimator(cfg),
		travel:  NewTravelEstimator(cfg, rdb),
		queue:   NewQueueEstimator(cfg),
		rdb:     rdb,
	}
}

// Calculate runs the three estimators concurrently and returns a composed ETA.
// Each component falls back to a heuristic on failure. The result is sticky-cached
// for StickyETATTL (default 30s).
func (s *ETAService) Calculate(ctx context.Context, req *model.ETARequest) (*model.ETAResponse, error) {
	// Check sticky cache first.
	if s.rdb != nil {
		cached, err := s.getCached(ctx, req.OrderID)
		if err == nil && cached != nil {
			cached.Cached = true
			return cached, nil
		}
	}

	calcCtx, cancel := context.WithTimeout(ctx, s.config.ConcurrentTimeout)
	defer cancel()

	type pickRes struct {
		val float64
	}
	type travRes struct {
		val float64
		err error
	}
	type queueRes struct {
		val float64
	}

	pickCh := make(chan pickRes, 1)
	travCh := make(chan travRes, 1)
	queueCh := make(chan queueRes, 1)

	// Load factor derived from item count (baseline: 10 items = load 1.0).
	load := float64(req.ItemCount) / 10.0

	go func() {
		pickCh <- pickRes{val: s.picking.Estimate(req, load)}
	}()
	go func() {
		t, err := s.travel.Estimate(calcCtx, req)
		travCh <- travRes{val: t, err: err}
	}()
	go func() {
		queueCh <- queueRes{val: s.queue.Estimate(req)}
	}()

	comps := model.ETAComponents{}

	select {
	case r := <-pickCh:
		comps.PickingTime = r.val
	case <-calcCtx.Done():
		comps.PickingTime = s.fallbackPicking(req)
	}

	select {
	case r := <-travCh:
		if r.err != nil {
			comps.TravelTime = s.fallbackTravel(req)
		} else {
			comps.TravelTime = r.val
		}
	case <-calcCtx.Done():
		comps.TravelTime = s.fallbackTravel(req)
	}

	select {
	case r := <-queueCh:
		comps.QueueWaitTime = r.val
	case <-calcCtx.Done():
		comps.QueueWaitTime = s.fallbackQueue(req)
	}

	base := comps.PickingTime + comps.QueueWaitTime + comps.TravelTime

	p50 := math.Min(base, s.config.MaxETA)
	p80 := math.Min(base*(1+s.config.ConservativeBufferPct), s.config.MaxETA)
	p95 := math.Min(base*(1+2*s.config.ConservativeBufferPct), s.config.MaxETA)

	comps.Buffer = p95 - base
	if comps.Buffer < 0 {
		comps.Buffer = 0
	}

	resp := &model.ETAResponse{
		OrderID:      req.OrderID,
		P50:          math.Round(p50*100) / 100,
		P80:          math.Round(p80*100) / 100,
		P95:          math.Round(p95*100) / 100,
		Components:   comps,
		FormattedETA: formatETA(base),
		ComputedAt:   time.Now().UnixMilli(),
	}

	// Persist sticky cache.
	if s.rdb != nil {
		s.setCached(ctx, req.OrderID, resp)
	}

	return resp, nil
}

// GetETA retrieves a previously cached ETA for the given order.
func (s *ETAService) GetETA(ctx context.Context, orderID string) (*model.ETAResponse, error) {
	if s.rdb == nil {
		return nil, fmt.Errorf("cache not available")
	}
	cached, err := s.getCached(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		return nil, fmt.Errorf("no cached ETA for order %s", orderID)
	}
	cached.Cached = true
	return cached, nil
}

// --- fallbacks ---

func (s *ETAService) fallbackPicking(req *model.ETARequest) float64 {
	return s.config.HubOverheadFixed + float64(req.ItemCount)*s.config.PickingPerItemMean
}

func (s *ETAService) fallbackTravel(req *model.ETARequest) float64 {
	distance := haversine(req.HubLat, req.HubLng, req.CustLat, req.CustLng)
	return distance / s.config.TravelSpeedMean
}

func (s *ETAService) fallbackQueue(_ *model.ETARequest) float64 {
	return s.config.QueueBaseLatency
}

// --- sticky cache ---

func (s *ETAService) getCached(ctx context.Context, orderID string) (*model.ETAResponse, error) {
	val, err := s.rdb.Get(ctx, etaCachePrefix+orderID).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var resp model.ETAResponse
	if err := json.Unmarshal(val, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *ETAService) setCached(ctx context.Context, orderID string, resp *model.ETAResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = s.rdb.Set(ctx, etaCachePrefix+orderID, data, s.config.StickyETATTL).Err()
}

// formatETA renders a human-readable ETA string.
func formatETA(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("~%.0fs", seconds)
	}
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	if mins < 60 {
		if secs > 0 {
			return fmt.Sprintf("~%dm %ds", mins, secs)
		}
		return fmt.Sprintf("~%dm", mins)
	}
	hrs := mins / 60
	mins = mins % 60
	if mins > 0 {
		return fmt.Sprintf("~%dh %dm", hrs, mins)
	}
	return fmt.Sprintf("~%dh", hrs)
}
