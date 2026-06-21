package model

import "time"

// DriverState represents the current status of a delivery driver.
type DriverState string

const (
	DriverOffline    DriverState = "OFFLINE"
	DriverIdle       DriverState = "IDLE"
	DriverAssigned   DriverState = "ASSIGNED"
	DriverToHub      DriverState = "TO_HUB"
	DriverPicking    DriverState = "PICKING"
	DriverDelivering DriverState = "DELIVERING"
	DriverCompleted  DriverState = "COMPLETED"
)

// OrderStatus represents the lifecycle status of an order.
type OrderStatus string

const (
	OrderPending     OrderStatus = "PENDING"
	OrderAssigned    OrderStatus = "ASSIGNED"
	OrderDispatched  OrderStatus = "DISPATCHED"
	OrderDelivered   OrderStatus = "DELIVERED"
	OrderFailed      OrderStatus = "FAILED"
)

// Location holds geographic coordinates.
type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// OrderItem represents a single item within an order.
type OrderItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
}

// Order represents a q-commerce order to be dispatched.
type Order struct {
	ID               string      `json:"id"`
	CustomerID       string      `json:"customer_id"`
	Items            []OrderItem `json:"items"`
	Total            float64     `json:"total"`
	Status           OrderStatus `json:"status"`
	Priority         int         `json:"priority"`
	CreatedAt        time.Time   `json:"created_at"`
	DeliveryLocation Location    `json:"delivery_location"`
	HubID            string      `json:"hub_id"`
	AssignedDriverID string      `json:"assigned_driver_id,omitempty"`
	BatchID          string      `json:"batch_id,omitempty"`
}

// Driver represents a delivery driver.
type Driver struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Location        Location    `json:"location"`
	Status          DriverState `json:"status"`
	Load            int         `json:"load"`
	MaxLoad         int         `json:"max_load"`
	TotalDeliveries int         `json:"total_deliveries"`
	LastAssignedAt  time.Time   `json:"last_assigned_at,omitempty"`
	SkippedCount    int         `json:"skipped_count"`
}

// Assignment represents a matched order-driver pair.
type Assignment struct {
	ID         string     `json:"id"`
	OrderID    string     `json:"order_id"`
	DriverID   string     `json:"driver_id"`
	Status     string     `json:"status"` // PENDING, ACCEPTED, REJECTED, TIMEOUT, CANCELLED
	Score      float64    `json:"score"`
	Attempt    int        `json:"attempt"`
	CreatedAt  time.Time  `json:"created_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
}

// BatchConfig controls the batching behaviour for order dispatch.
type BatchConfig struct {
	MaxWindow time.Duration `json:"max_window"`
	MinOrders int           `json:"min_orders"`
}

// MatchScore holds the scoring weights used by the greedy matcher.
type MatchScore struct {
	W1 float64 `json:"w1"` // distance weight
	W2 float64 `json:"w2"` // ETA weight
	W3 float64 `json:"w3"` // load weight
	W4 float64 `json:"w4"` // priority weight
	W5 float64 `json:"w5"` // anti-starvation weight
}

// StateTransitionEvent records a single driver state transition.
type StateTransitionEvent struct {
	DriverID  string      `json:"driver_id"`
	FromState DriverState `json:"from_state"`
	ToState   DriverState `json:"to_state"`
	Timestamp time.Time   `json:"timestamp"`
	Reason    string      `json:"reason,omitempty"`
}

// DispatchRequest is the POST /dispatch/order payload.
type DispatchRequest struct {
	CustomerID       string      `json:"customer_id"  binding:"required"`
	Items            []OrderItem `json:"items"         binding:"required,min=1,dive"`
	Total            float64     `json:"total"         binding:"required"`
	DeliveryLocation *Location   `json:"delivery_location" binding:"required"`
	HubID            string      `json:"hub_id"        binding:"required"`
	Priority         int         `json:"priority"`
}

// LocationUpdateRequest is the POST /dispatch/driver/location payload.
type LocationUpdateRequest struct {
	DriverID string  `json:"driver_id" binding:"required"`
	Lat      float64 `json:"lat"       binding:"required"`
	Lng      float64 `json:"lng"       binding:"required"`
}

// AcceptRequest is the POST /dispatch/driver/accept payload.
type AcceptRequest struct {
	AssignmentID string `json:"assignment_id" binding:"required"`
	DriverID     string `json:"driver_id"     binding:"required"`
}

// RejectRequest is the POST /dispatch/driver/reject payload.
type RejectRequest struct {
	AssignmentID string `json:"assignment_id" binding:"required"`
	DriverID     string `json:"driver_id"     binding:"required"`
	OrderID      string `json:"order_id"      binding:"required"`
}

// DefaultMatchScore returns sensible scoring defaults.
func DefaultMatchScore() MatchScore {
	return MatchScore{W1: 0.3, W2: 0.3, W3: 0.2, W4: 0.1, W5: 0.1}
}

// DefaultBatchConfig returns sensible batching defaults.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{MaxWindow: 10 * time.Second, MinOrders: 5}
}
