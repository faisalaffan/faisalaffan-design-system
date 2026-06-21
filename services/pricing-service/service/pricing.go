package service

import (
	"context"
	"math"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/model"
)

// ---- interfaces for pluggable dependencies ----

// SignalCollectorI fetches real-time supply/demand signals for an area.
type SignalCollectorI interface {
	Collect(ctx context.Context, areaID string) (*model.AreaSignals, error)
}

// SurgeDetectorI calculates the surge multiplier from a load ratio.
type SurgeDetectorI interface {
	Detect(areaID string, loadRatio float64) float64
}

// PriceLock persists and retrieves locked delivery fees.
type PriceLock interface {
	Set(ctx context.Context, orderID string, fee float64) error
	Get(ctx context.Context, orderID string) (float64, error)
}

// ElasticityTrackerI measures demand elasticity and suggests a surge cap.
type ElasticityTrackerI interface {
	Record(areaID string, loadRatio, surgeMult float64)
	SurgeCap(areaID string) float64
}

// ABTester assigns experiment variants to users.
type ABTester interface {
	GetVariant(userID, experimentName string) string
}

// ---- hub location ----

type hubLocation struct {
	Lat, Lng float64
}

var defaultHubs = map[string]hubLocation{
	"hub_jkt_01": {Lat: -6.2088, Lng: 106.8456},
	"hub_jkt_02": {Lat: -6.2250, Lng: 106.9000},
	"hub_bdg_01": {Lat: -6.9175, Lng: 107.6191},
	"hub_sby_01": {Lat: -7.2575, Lng: 112.7521},
	"hub_jog_01": {Lat: -7.7956, Lng: 110.3695},
	"hub_dps_01": {Lat: -8.6500, Lng: 115.2167},
}

// ---- PricingService ----

// PricingService orchestrates signal collection, surge detection, fee
// calculation, price locking, elasticity tracking, and A/B testing.
type PricingService struct {
	collector  SignalCollectorI
	detector   SurgeDetectorI
	lock       PriceLock
	elasticity ElasticityTrackerI
	abTest     ABTester
	hubs       map[string]hubLocation
}

// NewPricingService creates a new PricingService.
func NewPricingService(
	collector SignalCollectorI,
	detector SurgeDetectorI,
	lock PriceLock,
	elasticity ElasticityTrackerI,
	abTest ABTester,
) *PricingService {
	h := make(map[string]hubLocation, len(defaultHubs))
	for k, v := range defaultHubs {
		h[k] = v
	}
	return &PricingService{
		collector:  collector,
		detector:   detector,
		lock:       lock,
		elasticity: elasticity,
		abTest:     abTest,
		hubs:       h,
	}
}

// Constants for fee calculation.
const (
	BaseFee         = 5000.0 // IDR
	DistanceRate    = 2000.0 // IDR per km
	MinFee          = 5000.0 // IDR
	MaxFee          = 75000.0 // IDR
	MaxTotalMult    = 5.0    // total multiplier cap
	WeatherMult     = 1.2    // when raining
	PeakMult        = 1.3    // during peak hours
)

// Calculate computes the delivery fee by collecting signals, detecting surge,
// applying weather/peak multipliers, calculating distance (haversine), and
// locking the final price.
func (s *PricingService) Calculate(ctx context.Context, req *model.FeeRequest) (*model.FeeResponse, error) {
	// 1. Check for existing locked price
	if locked, err := s.lock.Get(ctx, req.OrderID); err == nil && locked > 0 {
		return &model.FeeResponse{
			DeliveryFee: locked,
			Message:     "locked price applied",
		}, nil
	}

	// 2. Collect signals
	signals, err := s.collector.Collect(ctx, req.AreaID)
	if err != nil {
		signals = &model.AreaSignals{CollectedAt: time.Now()}
	}

	// 3. Load ratio
	loadRatio := float64(signals.PendingOrders) / float64(max(signals.DriverCount, 1))

	// 4. Surge multiplier
	surgeMult := s.detector.Detect(req.AreaID, loadRatio)

	// 5. Elasticity-based surge cap
	elasticCap := s.elasticity.SurgeCap(req.AreaID)
	if surgeMult > elasticCap {
		surgeMult = elasticCap
	}
	s.elasticity.Record(req.AreaID, loadRatio, surgeMult)

	// 6. Weather multiplier
	wMult := 1.0
	if signals.IsRaining {
		wMult = WeatherMult
	}

	// 7. Peak hour multiplier
	pMult := 1.0
	if signals.IsPeakHour {
		pMult = PeakMult
	}

	// 8. Total multiplier (capped)
	totalMult := math.Min(surgeMult*wMult*pMult, MaxTotalMult)

	// 9. Distance via haversine
	hubLat, hubLng := s.resolveHub(req.HubID)
	dist := haversine(hubLat, hubLng, req.DestLat, req.DestLng)
	distFee := math.Round(dist * DistanceRate)

	// 10. Final fee
	subtotal := BaseFee + distFee
	finalFee := math.Round(subtotal * totalMult)
	finalFee = math.Max(finalFee, MinFee)
	finalFee = math.Min(finalFee, MaxFee)

	// 11. Lock price
	_ = s.lock.Set(ctx, req.OrderID, finalFee) // fail-open

	// 12. A/B test assignment (logged, does not affect fee)
	_ = s.abTest.GetVariant(req.UserID, "surge_v2")

	return &model.FeeResponse{
		DeliveryFee: finalFee,
		Breakdown: model.FeeBreakdown{
			BaseFee:           BaseFee,
			DistanceFee:       distFee,
			Subtotal:          subtotal,
			SurgeMultiplier:   surgeMult,
			WeatherMultiplier: wMult,
			PeakMultiplier:    pMult,
			FinalFee:          finalFee,
		},
		Message: "fee calculated successfully",
	}, nil
}

// GetLockedPrice retrieves a locked price for payment verification.
func (s *PricingService) GetLockedPrice(ctx context.Context, orderID string) (float64, error) {
	return s.lock.Get(ctx, orderID)
}

// GetSignals returns the current signals for an area (admin use).
func (s *PricingService) GetSignals(ctx context.Context, areaID string) (*model.AreaSignals, error) {
	return s.collector.Collect(ctx, areaID)
}

// ---- helpers ----

func (s *PricingService) resolveHub(hubID string) (lat, lng float64) {
	if loc, ok := s.hubs[hubID]; ok {
		return loc.Lat, loc.Lng
	}
	// Default to Jakarta hub
	return -6.2088, 106.8456
}

// haversine calculates the great-circle distance in km between two coordinates.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0 // Earth radius in km

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}
