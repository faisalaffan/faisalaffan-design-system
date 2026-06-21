// Package service implements geo-serviceability business logic
// with H3 hexagon indexing, haversine distance, tie-breaking, and caching.
package service

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/geo-service/model"
	"github.com/uber/h3-go/v4"
)

const (
	h3Resolution = 9
	earthRadiusR = 6371.0 // Earth mean radius in km
	baseETAMin   = 7
	speedKmh     = 25.0
)

// Service handles geo-serviceability business logic.
type Service struct {
	hubs  model.HubRepository
	cache *cache
}

// New creates a new Service with the given hub repository.
func New(hubs model.HubRepository) *Service {
	return &Service{
		hubs:  hubs,
		cache: newCache(1 * time.Hour),
	}
}

// CheckServiceability checks whether a GPS coordinate is serviceable by any hub.
// Returns a ServiceabilityResponse with the best hub, distance, ETA, and cache status.
func (s *Service) CheckServiceability(lat, lng float64) (*model.ServiceabilityResponse, error) {
	// Convert GPS to H3 resolution-9 cell
	cell, err := h3.LatLngToCell(h3.LatLng{Lat: lat, Lng: lng}, h3Resolution)
	if err != nil {
		return nil, fmt.Errorf("h3 latlng to cell: %w", err)
	}
	cellToken := cell.String()
	cacheKey := "geo:h3:" + cellToken

	// Cache hit — return cached best hub
	if hit := s.cacheGet(cacheKey, cellToken, lat, lng); hit != nil {
		return hit, nil
	}

	// Cache miss — compute best hub across all active hubs
	activeHubs := s.hubs.Active()
	bestHub, distance := s.GetBestHub(lat, lng, activeHubs)

	if bestHub == nil {
		return &model.ServiceabilityResponse{
			Serviceable: false,
			H3Cell:      cellToken,
			CacheHit:    false,
		}, nil
	}

	resp := &model.ServiceabilityResponse{
		Serviceable: true,
		Hub:         bestHub,
		H3Cell:      cellToken,
		DistanceKm:  round2(distance),
		ETA:         formatETA(distance),
		CacheHit:    false,
		Score:       round3(computeScore(distance, bestHub, model.DefaultTieBreak())),
	}

	s.cache.set(cacheKey, bestHub.ID)
	return resp, nil
}

// GetBestHub scores all given hubs using tie-breaking weights and
// returns the best hub along with its haversine distance from the point.
// Only hubs within their MaxRadiusKm are considered.
func (s *Service) GetBestHub(lat, lng float64, hubs []*model.Hub) (*model.Hub, float64) {
	tb := model.DefaultTieBreak()
	var best *model.Hub
	var bestDist float64
	var bestScore float64

	for _, h := range hubs {
		if !h.Active {
			continue
		}
		dist := haversine(lat, lng, h.Lat, h.Lng)
		if dist > h.MaxRadiusKm {
			continue
		}
		score := computeScore(dist, h, tb)
		if best == nil || score > bestScore {
			best = h
			bestDist = dist
			bestScore = score
		}
	}
	return best, bestDist
}

// cacheGet returns a cached ServiceabilityResponse if the H3 cell has a
// cached best hub that is still active.
func (s *Service) cacheGet(cacheKey, cellToken string, lat, lng float64) *model.ServiceabilityResponse {
	hubID, ok := s.cache.get(cacheKey)
	if !ok {
		return nil
	}
	hub, exists := s.hubs.GetByID(hubID)
	if !exists || !hub.Active {
		return nil
	}
	dist := haversine(lat, lng, hub.Lat, hub.Lng)
	return &model.ServiceabilityResponse{
		Serviceable: true,
		Hub:         hub,
		H3Cell:      cellToken,
		DistanceKm:  round2(dist),
		ETA:         formatETA(dist),
		CacheHit:    true,
		Score:       round3(computeScore(dist, hub, model.DefaultTieBreak())),
	}
}

// computeScore calculates the composite tie-breaking score for a hub:
//   40% distance (closer = better) + 40% stock + 20% rider availability.
func computeScore(distance float64, h *model.Hub, tb model.TieBreakScore) float64 {
	distNorm := 1.0 - (distance / h.MaxRadiusKm)
	if distNorm < 0 {
		distNorm = 0
	}
	return tb.DistanceWeight*distNorm +
		tb.StockWeight*h.Stock +
		tb.RiderWeight*h.RiderAvail
}

// haversine calculates the great-circle distance between two GPS coordinates
// using the Haversine formula. Returns distance in kilometers.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	rLat1 := lat1 * math.Pi / 180
	rLat2 := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusR * c
}

// formatETA returns an ETA string: 7 minutes base + (distance / 25 km/h) in minutes.
func formatETA(distKm float64) string {
	travelMin := int(math.Ceil(distKm / speedKmh * 60))
	return fmt.Sprintf("%dm", baseETAMin+travelMin)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// ---------------------------------------------------------------------------
// In-memory TTL cache for H3 cell → hub ID mappings.
// This implements the caching layer described in the architecture; a Redis
// backend can be swapped in by implementing the same get/set contract.
// ---------------------------------------------------------------------------

type cache struct {
	mu   sync.RWMutex
	data map[string]cacheEntry
	ttl  time.Duration
}

type cacheEntry struct {
	hubID     string
	expiresAt time.Time
}

func newCache(ttl time.Duration) *cache {
	c := &cache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
	}
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.evictExpired()
		}
	}()
	return c
}

func (c *cache) get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.hubID, true
}

func (c *cache) set(key, hubID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cacheEntry{
		hubID:     hubID,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *cache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.data {
		if now.After(v.expiresAt) {
			delete(c.data, k)
		}
	}
}
