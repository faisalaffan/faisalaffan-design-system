package service

import (
	"math"
	"sort"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
	"github.com/google/uuid"
)

const (
	earthRadiusKm = 6371.0
	avgSpeedMps   = 8.33 // ~30 km/h in m/s
)

// GreedyMatcher performs greedy driver-order assignment using a configurable
// weighted scoring function with anti-starvation support.
type GreedyMatcher struct {
	config model.MatchScore
}

// NewGreedyMatcher creates a new GreedyMatcher with the given weights.
func NewGreedyMatcher(cfg model.MatchScore) *GreedyMatcher {
	return &GreedyMatcher{config: cfg}
}

// Haversine returns the great-circle distance in kilometres between two
// geographic coordinates using the Haversine formula.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	r1 := lat1 * math.Pi / 180
	r2 := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(r1)*math.Cos(r2)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ETA returns the estimated time of arrival in seconds given a distance in
// kilometres and an assumed average speed of 8.33 m/s (~30 km/h).
func ETA(distKm float64) float64 {
	return distKm / avgSpeedMps * 1000
}

// score computes the composite match score between one order and one driver.
// Lower is better (more desirable match).
//
//	score = w1*normalizedDist + w2*normalizedETA + w3*loadFactor +
//	        w4*priorityBonus + w5*starvationPenalty
func (m *GreedyMatcher) score(order *model.Order, driver *model.Driver) float64 {
	dist := Haversine(
		order.DeliveryLocation.Lat, order.DeliveryLocation.Lng,
		driver.Location.Lat, driver.Location.Lng,
	)
	eta := ETA(dist)

	// Normalise distance (assume max useful distance is 10 km).
	normDist := math.Min(dist/10.0, 1.0)
	// Normalise ETA (assume max useful ETA is 1200 s / 20 min).
	normETA := math.Min(eta/1200.0, 1.0)

	// Driver load as a fraction of capacity.
	loadFactor := float64(driver.Load) / float64(max(driver.MaxLoad, 1))

	// Priority bonus: priority 1 (highest urgency) gets the largest bonus
	// (negative score reduction), making the order more attractive.
	// Each level above 1 reduces the bonus by 0.125.
	priorityBonus := -float64(max(order.Priority, 1)-1) * 0.125

	// Starvation penalty: drivers that have been skipped accumulate a
	// negative score adjustment, making them more likely to be picked.
	starvationPenalty := math.Min(float64(driver.SkippedCount)*0.1, 0.5)

	return m.config.W1*normDist +
		m.config.W2*normETA +
		m.config.W3*loadFactor +
		m.config.W4*priorityBonus +
		m.config.W5*(-starvationPenalty)
}

// Match performs greedy assignment of a batch of orders to the available
// drivers. It builds a score matrix, picks the lowest-score pair, assigns,
// removes both from the pool, and repeats. After each round, every unassigned
// driver gets its SkippedCount incremented (anti-starvation).
func (m *GreedyMatcher) Match(orders []*model.Order, drivers []*model.Driver) []model.Assignment {
	if len(orders) == 0 || len(drivers) == 0 {
		return nil
	}

	avail := make([]*model.Driver, len(drivers))
	copy(avail, drivers)

	remaining := make([]*model.Order, len(orders))
	copy(remaining, orders)

	var assignments []model.Assignment
	now := time.Now()

	for len(remaining) > 0 && len(avail) > 0 {
		bestOi, bestDi, bestScore := -1, -1, math.MaxFloat64

		for oi, o := range remaining {
			for di, d := range avail {
				s := m.score(o, d)
				if s < bestScore {
					bestScore = s
					bestOi = oi
					bestDi = di
				}
			}
		}

		if bestOi == -1 || bestDi == -1 {
			break
		}

		driver := avail[bestDi]
		order := remaining[bestOi]

		order.Status = model.OrderAssigned
		order.AssignedDriverID = driver.ID
		driver.Status = model.DriverAssigned
		driver.Load++
		driver.LastAssignedAt = now
		driver.SkippedCount = 0

		assignments = append(assignments, model.Assignment{
			ID:        uuid.New().String(),
			OrderID:   order.ID,
			DriverID:  driver.ID,
			Status:    "PENDING",
			Score:     bestScore,
			Attempt:   1,
			CreatedAt: now,
		})

		// Remove assigned pair from the pool.
		remaining = append(remaining[:bestOi], remaining[bestOi+1:]...)
		avail = append(avail[:bestDi], avail[bestDi+1:]...)

		// Anti-starvation: increment skip counter for every remaining driver.
		for _, d := range avail {
			d.SkippedCount++
		}
	}

	return assignments
}

// FindBestDriver returns the single best driver for a given order along with
// its match score. Returns nil if no drivers are available.
func (m *GreedyMatcher) FindBestDriver(order *model.Order, drivers []*model.Driver) (*model.Driver, float64) {
	if len(drivers) == 0 {
		return nil, 0
	}
	bestIdx := -1
	bestScore := math.MaxFloat64
	for i, d := range drivers {
		s := m.score(order, d)
		if s < bestScore {
			bestScore = s
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return nil, 0
	}
	return drivers[bestIdx], bestScore
}

// SortByScore returns drivers sorted by their match score (ascending, best
// first) for the given order.
func (m *GreedyMatcher) SortByScore(order *model.Order, drivers []*model.Driver) []*model.Driver {
	type scored struct {
		d *model.Driver
		s float64
	}
	scoredSlice := make([]scored, len(drivers))
	for i, d := range drivers {
		scoredSlice[i] = scored{d, m.score(order, d)}
	}
	sort.Slice(scoredSlice, func(i, j int) bool {
		return scoredSlice[i].s < scoredSlice[j].s
	})
	result := make([]*model.Driver, len(drivers))
	for i, s := range scoredSlice {
		result[i] = s.d
	}
	return result
}
