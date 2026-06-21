package model

import "time"

// FeeRequest is the input for delivery fee calculation.
type FeeRequest struct {
	OrderID    string  `json:"order_id" binding:"required"`
	UserID     string  `json:"user_id" binding:"required"`
	AreaID     string  `json:"area_id" binding:"required"`
	HubID      string  `json:"hub_id" binding:"required"`
	DestLat    float64 `json:"dest_lat" binding:"required"`
	DestLng    float64 `json:"dest_lng" binding:"required"`
	OrderTotal float64 `json:"order_total" binding:"required"`
}

// FeeResponse is the output of delivery fee calculation.
type FeeResponse struct {
	DeliveryFee float64      `json:"delivery_fee"`
	Breakdown   FeeBreakdown `json:"breakdown"`
	Message     string       `json:"message"`
}

// FeeBreakdown contains all components of the delivery fee.
type FeeBreakdown struct {
	BaseFee           float64 `json:"base_fee"`
	DistanceFee       float64 `json:"distance_fee"`
	Subtotal          float64 `json:"subtotal"`
	SurgeMultiplier   float64 `json:"surge_multiplier"`
	WeatherMultiplier float64 `json:"weather_multiplier"`
	PeakMultiplier    float64 `json:"peak_multiplier"`
	FinalFee          float64 `json:"final_fee"`
}

// AreaSignals represents real-time supply/demand signals for an area.
type AreaSignals struct {
	DriverCount   int       `json:"driver_count"`
	PendingOrders int       `json:"pending_orders"`
	LoadRatio     float64   `json:"load_ratio"`
	IsRaining     bool      `json:"is_raining"`
	IsPeakHour    bool      `json:"is_peak_hour"`
	CollectedAt   time.Time `json:"collected_at"`
}

// SurgeConfig defines cascading surge thresholds and multipliers.
type SurgeConfig struct {
	LowThreshold       float64 `json:"low_threshold"`
	LowMultiplier      float64 `json:"low_multiplier"`
	MediumThreshold    float64 `json:"medium_threshold"`
	MediumMultiplier   float64 `json:"medium_multiplier"`
	HighThreshold      float64 `json:"high_threshold"`
	HighMultiplier     float64 `json:"high_multiplier"`
	CriticalThreshold  float64 `json:"critical_threshold"`
	CriticalMultiplier float64 `json:"critical_multiplier"`
}

// DefaultSurgeConfig returns the default surge config with cascading thresholds:
//
//	1.0  → 1.2x
//	2.0  → 1.5x
//	3.5  → 2.0x
//	5.0  → 3.0x
func DefaultSurgeConfig() SurgeConfig {
	return SurgeConfig{
		LowThreshold:      1.0,
		LowMultiplier:     1.2,
		MediumThreshold:   2.0,
		MediumMultiplier:  1.5,
		HighThreshold:     3.5,
		HighMultiplier:    2.0,
		CriticalThreshold: 5.0,
		CriticalMultiplier: 3.0,
	}
}
