package service

import (
	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
)

// QueueEstimator computes queue wait time based on queue depth and driver availability.
type QueueEstimator struct {
	config model.ETAConfig
}

// NewQueueEstimator creates a new QueueEstimator.
func NewQueueEstimator(cfg model.ETAConfig) *QueueEstimator {
	return &QueueEstimator{config: cfg}
}

// Estimate returns queue wait time in seconds.
// queue_wait = max(queue_depth / max(drivers_avail × 1.5, 1), 1) × dispatch_latency
func (e *QueueEstimator) Estimate(req *model.ETARequest) float64 {
	effectiveDrivers := float64(req.DriversAvail) * 1.5
	if effectiveDrivers < 1 {
		effectiveDrivers = 1
	}

	ratio := float64(req.QueueDepth) / effectiveDrivers
	if ratio < 1 {
		ratio = 1
	}
	return ratio * e.config.DispatchLatency
}
