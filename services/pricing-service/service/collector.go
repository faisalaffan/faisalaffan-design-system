package service

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/model"
)

// SignalCollectorConfig configures the signal collector timeout.
type SignalCollectorConfig struct {
	Timeout time.Duration
}

// SignalCollector fetches real-time signals concurrently with error tolerance.
// On any fetch failure, the field is left at its zero value.
type SignalCollector struct {
	timeout        time.Duration
	fetchDriversFn func(ctx context.Context, areaID string) (int, error)
	fetchPendingFn func(ctx context.Context, areaID string) (int, error)
	fetchWeatherFn func(ctx context.Context, areaID string) (bool, error)
}

// NewSignalCollector creates a new SignalCollector. Default timeout is 5 seconds.
func NewSignalCollector(cfg SignalCollectorConfig) *SignalCollector {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &SignalCollector{
		timeout:        cfg.Timeout,
		fetchDriversFn: defaultFetchDrivers,
		fetchPendingFn: defaultFetchPendingOrders,
		fetchWeatherFn: defaultFetchWeather,
	}
}

// Collect fetches all signals concurrently and returns aggregated area signals.
// Error tolerance: failed fetches leave the field at its zero value.
func (c *SignalCollector) Collect(ctx context.Context, areaID string) (*model.AreaSignals, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var (
		drivers int
		pending int
		raining bool
	)
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		d, err := c.fetchDriversFn(ctx, areaID)
		if err != nil {
			return
		}
		mu.Lock()
		drivers = d
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p, err := c.fetchPendingFn(ctx, areaID)
		if err != nil {
			return
		}
		mu.Lock()
		pending = p
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := c.fetchWeatherFn(ctx, areaID)
		if err != nil {
			return
		}
		mu.Lock()
		raining = r
		mu.Unlock()
	}()

	wg.Wait()

	peak := isPeakHour(time.Now())

	loadRatio := 0.0
	if drivers > 0 {
		loadRatio = float64(pending) / float64(drivers)
	}

	return &model.AreaSignals{
		DriverCount:   drivers,
		PendingOrders: pending,
		LoadRatio:     loadRatio,
		IsRaining:     raining,
		IsPeakHour:    peak,
		CollectedAt:   time.Now(),
	}, nil
}

// isPeakHour returns true during meal windows.
func isPeakHour(t time.Time) bool {
	h := t.Hour()
	return (h >= 7 && h <= 9) || (h >= 12 && h <= 14) || (h >= 18 && h <= 21)
}

// --- default simulated fetchers ---

func defaultFetchDrivers(ctx context.Context, areaID string) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	base := map[string]int{
		"area_jkt": 50,
		"area_bdg": 30,
		"area_sby": 40,
		"area_jog": 20,
		"area_dps": 25,
	}[areaID]
	if base == 0 {
		base = 15
	}
	h := time.Now().Hour()
	switch {
	case h < 6 || h >= 22:
		base = base * 3 / 10
	case h < 8 || h >= 17:
		base = base * 7 / 10
	}
	return max(base, 1), nil
}

func defaultFetchPendingOrders(ctx context.Context, areaID string) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	base := map[string]int{
		"area_jkt": 80,
		"area_bdg": 45,
		"area_sby": 60,
		"area_jog": 25,
		"area_dps": 20,
	}[areaID]
	if base == 0 {
		base = 20
	}
	h := time.Now().Hour()
	switch {
	case h >= 11 && h <= 13:
		base = base * 15 / 10
	case h >= 18 && h <= 20:
		base = base * 18 / 10
	case h < 6 || h >= 22:
		base = base * 3 / 10
	}
	jitter := rand.Intn(max(base/4, 1))
	return base + jitter, nil
}

func defaultFetchWeather(ctx context.Context, areaID string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	chance := 0.15
	switch areaID {
	case "area_jkt", "area_bdg":
		chance = 0.30
	case "area_sby":
		chance = 0.20
	}
	h := time.Now().Hour()
	if h >= 14 && h <= 17 {
		chance *= 1.5
	}
	return rand.Float64() < chance, nil
}
