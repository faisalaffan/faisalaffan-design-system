package service

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
)

var (
	ErrInvalidLat        = errors.New("latitude out of range ±90")
	ErrInvalidLng        = errors.New("longitude out of range ±180")
	ErrSpeedTooHigh      = errors.New("speed exceeds maximum threshold")
	ErrInvalidAccuracy   = errors.New("accuracy out of valid range 0-500m")
	ErrStaleLocation     = errors.New("location timestamp is too old")
	ErrFutureTimestamp   = errors.New("location timestamp is in the future")
	ErrThrottled         = errors.New("update too frequent, throttled")
	ErrDuplicate         = errors.New("location unchanged within duplicate threshold")
)

// LocationValidator validates raw GPS updates against a ruleset.
type LocationValidator struct {
	rules       model.ValidationRules
	lastUpdate  map[string]time.Time // driverID -> last accepted timestamp
	lastPos     map[string]geoPoint  // driverID -> last accepted point
	mu          sync.Mutex
}

type geoPoint struct {
	lat float64
	lng float64
}

// NewLocationValidator creates a validator with the given rules.
func NewLocationValidator(rules model.ValidationRules) *LocationValidator {
	return &LocationValidator{
		rules:      rules,
		lastUpdate: make(map[string]time.Time),
		lastPos:    make(map[string]geoPoint),
	}
}

// Validate runs the full validation pipeline against a location update.
// Returns the first validation error encountered.
func (v *LocationValidator) Validate(u model.LocationUpdate) error {
	if err := v.validateFormat(u); err != nil {
		return err
	}
	if err := v.validateStaleness(u); err != nil {
		return err
	}
	if err := v.validateThrottle(u); err != nil {
		return err
	}
	if err := v.validateDuplicate(u); err != nil {
		return err
	}
	// Accept the update — record it for future throttle/duplicate checks.
	v.mu.Lock()
	v.lastUpdate[u.DriverID] = u.Timestamp
	v.lastPos[u.DriverID] = geoPoint{lat: u.Lat, lng: u.Lng}
	v.mu.Unlock()
	return nil
}

// validateFormat checks bounds and basic sensor sanity.
func (v *LocationValidator) validateFormat(u model.LocationUpdate) error {
	if u.Lat < v.rules.MinLat || u.Lat > v.rules.MaxLat {
		return ErrInvalidLat
	}
	if u.Lng < v.rules.MinLng || u.Lng > v.rules.MaxLng {
		return ErrInvalidLng
	}
	if u.Speed < 0 || u.Speed > v.rules.MaxSpeed {
		return ErrSpeedTooHigh
	}
	if u.Accuracy < 0 || u.Accuracy > v.rules.MaxAccuracy {
		return ErrInvalidAccuracy
	}
	return nil
}

// validateStaleness rejects old or future timestamps.
func (v *LocationValidator) validateStaleness(u model.LocationUpdate) error {
	now := time.Now()
	if u.Timestamp.After(now.Add(5 * time.Second)) {
		return ErrFutureTimestamp
	}
	if now.Sub(u.Timestamp) > v.rules.StalenessLimit {
		return ErrStaleLocation
	}
	return nil
}

// validateThrottle enforces minimum interval between updates from the same driver.
func (v *LocationValidator) validateThrottle(u model.LocationUpdate) error {
	v.mu.Lock()
	last, ok := v.lastUpdate[u.DriverID]
	v.mu.Unlock()
	if !ok {
		return nil
	}
	if u.Timestamp.Sub(last) < v.rules.ThrottleInterval {
		return ErrThrottled
	}
	return nil
}

// validateDuplicate drops updates that haven't moved meaningfully.
func (v *LocationValidator) validateDuplicate(u model.LocationUpdate) error {
	v.mu.Lock()
	last, ok := v.lastPos[u.DriverID]
	v.mu.Unlock()
	if !ok {
		return nil
	}
	d := haversine(last.lat, last.lng, u.Lat, u.Lng)
	if d < v.rules.DuplicateDistance {
		return ErrDuplicate
	}
	return nil
}

// haversine computes the great-circle distance in meters between two lat/lng points.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6_371_000.0 // Earth radius in metres
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2.0 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
