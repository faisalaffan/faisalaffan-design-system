package service

import (
	"math"
	"sync"
)

// elasticityPoint pairs a price (surge multiplier) with a quantity (load ratio).
type elasticityPoint struct {
	price    float64
	quantity float64
}

// ElasticityTracker tracks price elasticity per area using log-log regression.
// High absolute slope = elastic → low surge cap (1.5x).
// Low absolute slope = inelastic → high surge cap (5.0x).
type ElasticityTracker struct {
	mu        sync.RWMutex
	data      map[string][]elasticityPoint
	maxPoints int
}

// NewElasticityTracker creates a new ElasticityTracker.
func NewElasticityTracker() *ElasticityTracker {
	return &ElasticityTracker{
		data:      make(map[string][]elasticityPoint),
		maxPoints: 100,
	}
}

// Record stores a price-quantity observation for an area.
func (t *ElasticityTracker) Record(areaID string, loadRatio, surgeMult float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	pts := t.data[areaID]
	pts = append(pts, elasticityPoint{price: surgeMult, quantity: loadRatio})
	if len(pts) > t.maxPoints {
		pts = pts[len(pts)-t.maxPoints:]
	}
	t.data[areaID] = pts
}

// SurgeCap returns the maximum surge multiplier for an area based on its
// measured price elasticity. Returns 5.0 (inelastic default) when insufficient
// data is available.
func (t *ElasticityTracker) SurgeCap(areaID string) float64 {
	t.mu.RLock()
	pts := t.data[areaID]
	t.mu.RUnlock()

	if len(pts) < 2 {
		return 5.0
	}

	slope := logLogRegression(pts)
	absSlope := math.Abs(slope)

	switch {
	case absSlope > 1.5:
		return 1.5
	case absSlope > 1.0:
		return 2.0
	case absSlope > 0.5:
		return 3.0
	default:
		return 5.0
	}
}

// logLogRegression computes the slope of ln(quantity) against ln(price).
// Returns 0 when the denominator is zero or data is degenerate.
func logLogRegression(pts []elasticityPoint) float64 {
	var n, sumX, sumY, sumXY, sumX2 float64

	for _, p := range pts {
		if p.price <= 0 || p.quantity <= 0 {
			continue
		}
		logX := math.Log(p.price)
		logY := math.Log(p.quantity)
		sumX += logX
		sumY += logY
		sumXY += logX * logY
		sumX2 += logX * logX
		n++
	}

	if n < 2 {
		return 0
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denom
}
