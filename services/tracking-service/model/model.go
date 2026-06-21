package model

import "time"

// LocationUpdate is the raw GPS payload from a driver's device.
type LocationUpdate struct {
	OrderID   string    `json:"order_id"`
	DriverID  string    `json:"driver_id"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Speed     float64   `json:"speed"`     // m/s
	Bearing   float64   `json:"bearing"`   // degrees 0-360
	Accuracy  float64   `json:"accuracy"`  // meters (radius)
	Timestamp time.Time `json:"timestamp"`
}

// SmoothedPosition is the Kalman-filtered output.
type SmoothedPosition struct {
	OrderID  string    `json:"order_id"`
	DriverID string    `json:"driver_id"`
	Lat      float64   `json:"lat"`
	Lng      float64   `json:"lng"`
	Speed    float64   `json:"speed"`
	Bearing  float64   `json:"bearing"`
	Time     time.Time `json:"time"`
}

// ETAInfo wraps the current position with estimated arrival data.
type ETAInfo struct {
	Position    SmoothedPosition `json:"position"`
	ETA         time.Duration    `json:"eta"`
	Distance    float64          `json:"distance"` // meters remaining
	Status      OrderStatus      `json:"status"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// OrderStatus represents the lifecycle of a delivery order.
type OrderStatus string

const (
	StatusPreparing     OrderStatus = "preparing"
	StatusDriverAssigned OrderStatus = "driver_assigned"
	StatusInTransit      OrderStatus = "in_transit"
	StatusDelivered      OrderStatus = "delivered"
	StatusCancelled      OrderStatus = "cancelled"
)

// ValidationRules holds configurable thresholds for location validation.
type ValidationRules struct {
	MaxLat            float64       // +90
	MinLat            float64       // -90
	MaxLng            float64       // +180
	MinLng            float64       // -180
	MaxSpeed          float64       // m/s, e.g. 55.6 (~200 km/h)
	MaxAccuracy       float64       // meters
	StalenessLimit    time.Duration // max age of a location before rejection
	ThrottleInterval  time.Duration // minimum time between updates from same driver
	DuplicateDistance float64       // meters; updates closer than this are dropped
}

// DefaultValidationRules returns sensible defaults for q-commerce.
func DefaultValidationRules() ValidationRules {
	return ValidationRules{
		MaxLat:            90.0,
		MinLat:           -90.0,
		MaxLng:           180.0,
		MinLng:          -180.0,
		MaxSpeed:         55.6,
		MaxAccuracy:      500.0,
		StalenessLimit:   30 * time.Second,
		ThrottleInterval: 2 * time.Second,
		DuplicateDistance: 0.5,
	}
}

// SSEEvent represents the JSON payload sent over SSE.
type SSEEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}
