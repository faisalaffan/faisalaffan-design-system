package model

import "time"

// ETARequest represents an ETA calculation request.
type ETARequest struct {
	OrderID      string  `json:"order_id" binding:"required"`
	HubID        string  `json:"hub_id" binding:"required"`
	ItemCount    int     `json:"item_count" binding:"required,min=1"`
	HubLat       float64 `json:"hub_lat" binding:"required"`
	HubLng       float64 `json:"hub_lng" binding:"required"`
	CustLat      float64 `json:"cust_lat" binding:"required"`
	CustLng      float64 `json:"cust_lng" binding:"required"`
	QueueDepth   int     `json:"queue_depth" binding:"required,min=0"`
	DriversAvail int     `json:"drivers_avail" binding:"required,min=0"`
	Hour         int     `json:"hour"`
	Weekday      int     `json:"weekday"`
}

// ETAComponents holds the breakdown of the total ETA.
type ETAComponents struct {
	PickingTime   float64 `json:"picking_time_s"`
	QueueWaitTime float64 `json:"queue_wait_time_s"`
	TravelTime    float64 `json:"travel_time_s"`
	Buffer        float64 `json:"buffer_s"`
}

// ETAResponse is the response returned to the caller.
type ETAResponse struct {
	OrderID      string        `json:"order_id"`
	P50          float64       `json:"eta_p50_s"`
	P80          float64       `json:"eta_p80_s"`
	P95          float64       `json:"eta_p95_s"`
	Components   ETAComponents `json:"components"`
	FormattedETA string        `json:"formatted_eta"`
	Cached       bool          `json:"cached"`
	ComputedAt   int64         `json:"computed_at"`
}

// ETAConfig holds all tunable parameters for the estimation engine.
type ETAConfig struct {
	PickingPerItemMean    float64
	PickingPerItemStd     float64
	HubOverheadFixed      float64
	TravelSpeedMean       float64
	ConservativeBufferPct float64
	MaxETA                float64
	QueueBaseLatency      float64
	DispatchLatency       float64
	StickyETATTL          time.Duration
	ConcurrentTimeout     time.Duration
}

// DefaultConfig returns a sane default configuration.
func DefaultConfig() ETAConfig {
	return ETAConfig{
		PickingPerItemMean:    30,
		PickingPerItemStd:     10,
		HubOverheadFixed:      60,
		TravelSpeedMean:       8.33,
		ConservativeBufferPct: 0.20,
		MaxETA:                1800,
		QueueBaseLatency:      5,
		DispatchLatency:       2,
		StickyETATTL:          30 * time.Second,
		ConcurrentTimeout:     5 * time.Second,
	}
}
