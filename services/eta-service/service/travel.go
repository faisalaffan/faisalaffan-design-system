package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
	"github.com/redis/go-redis/v9"
)

// TravelEstimator computes travel time using haversine distance and traffic data.
type TravelEstimator struct {
	config model.ETAConfig
	rdb    *redis.Client
}

// NewTravelEstimator creates a new TravelEstimator.
func NewTravelEstimator(cfg model.ETAConfig, rdb *redis.Client) *TravelEstimator {
	return &TravelEstimator{config: cfg, rdb: rdb}
}

// haversine returns the great-circle distance in meters between two lat/lng points.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000 // earth radius in metres
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// Estimate returns travel time in seconds.
func (e *TravelEstimator) Estimate(ctx context.Context, req *model.ETARequest) (float64, error) {
	distance := haversine(req.HubLat, req.HubLng, req.CustLat, req.CustLng)
	baseTime := distance / e.config.TravelSpeedMean
	multiplier := e.trafficMultiplier(ctx, req.HubLat, req.HubLng, req.CustLat, req.CustLng)
	return baseTime * multiplier, nil
}

func routeCacheKey(hubLat, hubLng, custLat, custLng float64) string {
	return fmt.Sprintf("eta:travel:%.4f:%.4f:%.4f:%.4f", hubLat, hubLng, custLat, custLng)
}

func obsCacheKey(hubLat, hubLng, custLat, custLng float64) string {
	return fmt.Sprintf("eta:travel:obs:%.4f:%.4f:%.4f:%.4f", hubLat, hubLng, custLat, custLng)
}

// trafficMultiplier returns a traffic multiplier via sliding window (last 15 min).
// Falls back to 1.0 when no data is available.
func (e *TravelEstimator) trafficMultiplier(ctx context.Context, hubLat, hubLng, custLat, custLng float64) float64 {
	if e.rdb == nil {
		return 1.0
	}

	ck := routeCacheKey(hubLat, hubLng, custLat, custLng)

	// Try cached multiplier.
	cached, err := e.rdb.Get(ctx, ck).Float64()
	if err == nil {
		return clamp(cached, 0.5, 3.0)
	}

	// Compute from sliding window observations.
	ok := obsCacheKey(hubLat, hubLng, custLat, custLng)
	now := time.Now()
	minScore := now.Add(-15 * time.Minute).UnixMilli()
	maxScore := now.UnixMilli()

	// Clean stale entries.
	_ = e.rdb.ZRemRangeByScore(ctx, ok, "0", strconv.FormatInt(minScore, 10))

	entries, err := e.rdb.ZRangeByScore(ctx, ok, &redis.ZRangeBy{
		Min: strconv.FormatInt(minScore, 10),
		Max: strconv.FormatInt(maxScore, 10),
	}).Result()
	if err != nil || len(entries) == 0 {
		_ = e.rdb.Set(ctx, ck, 1.0, 5*time.Minute).Err()
		return 1.0
	}

	var total float64
	var count int
	for _, entry := range entries {
		// entry format: "nanotimestamp_multiplier"
		parts := strings.Split(entry, "_")
		if len(parts) >= 2 {
			mult, err := strconv.ParseFloat(parts[len(parts)-1], 64)
			if err == nil {
				total += mult
				count++
			}
		}
	}

	if count == 0 {
		_ = e.rdb.Set(ctx, ck, 1.0, 5*time.Minute).Err()
		return 1.0
	}

	avg := clamp(total/float64(count), 0.5, 3.0)
	_ = e.rdb.Set(ctx, ck, avg, 5*time.Minute).Err()
	return avg
}

// Observe records a travel time observation for the sliding window.
// Called externally when actual delivery times are known.
func (e *TravelEstimator) Observe(ctx context.Context, hubLat, hubLng, custLat, custLng float64, actualSeconds float64) error {
	if e.rdb == nil {
		return nil
	}

	distance := haversine(hubLat, hubLng, custLat, custLng)
	expected := distance / e.config.TravelSpeedMean
	if expected <= 0 {
		return nil
	}

	multiplier := actualSeconds / expected
	ok := obsCacheKey(hubLat, hubLng, custLat, custLng)

	member := fmt.Sprintf("%d_%s", time.Now().UnixNano(), strconv.FormatFloat(multiplier, 'f', 4, 64))
	_, err := e.rdb.ZAdd(ctx, ok, redis.Z{
		Score:  float64(time.Now().UnixMilli()),
		Member: member,
	}).Result()
	return err
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
