// Package model defines domain types for the geo-serviceability service.
package model

// Hub represents a fulfillment hub with geospatial data and operational metrics.
type Hub struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	MaxRadiusKm float64 `json:"max_radius_km"`
	Priority    int     `json:"priority"`
	Stock       float64 `json:"stock"`       // 0.0–1.0 normalized stock level
	RiderAvail  float64 `json:"rider_avail"` // 0.0–1.0 normalized rider availability
	Active      bool    `json:"active"`
}

// ServiceabilityRequest represents a user query for location serviceability.
type ServiceabilityRequest struct {
	Lat float64 `form:"lat" binding:"required"`
	Lng float64 `form:"lng" binding:"required"`
}

// ServiceabilityResponse represents the result of a serviceability check.
type ServiceabilityResponse struct {
	Serviceable bool    `json:"serviceable"`
	Hub         *Hub    `json:"hub,omitempty"`
	H3Cell      string  `json:"h3_cell,omitempty"`
	DistanceKm  float64 `json:"distance_km,omitempty"`
	ETA         string  `json:"eta,omitempty"`
	CacheHit    bool    `json:"cache_hit"`
	Score       float64 `json:"score,omitempty"`
}

// TieBreakScore defines the weight distribution for hub tie-breaking.
type TieBreakScore struct {
	DistanceWeight float64
	StockWeight    float64
	RiderWeight    float64
}

// DefaultTieBreak returns the default tie-breaking weights (40/40/20).
func DefaultTieBreak() TieBreakScore {
	return TieBreakScore{
		DistanceWeight: 0.4,
		StockWeight:    0.4,
		RiderWeight:    0.2,
	}
}

// HubRepository defines the interface for hub data access.
// Implementations must be safe for concurrent use.
type HubRepository interface {
	Add(hub *Hub) error
	GetAll() []*Hub
	GetByID(id string) (*Hub, bool)
	Active() []*Hub
}
