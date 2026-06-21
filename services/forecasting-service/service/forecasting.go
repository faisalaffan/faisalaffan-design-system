package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

const (
	// MaxConcurrency limits the number of parallel forecast computations.
	MaxConcurrency = 20
	// PerPairTimeout is the max time spent forecasting a single SKU-hub pair.
	PerPairTimeout = 30 * time.Second
	// historyDays is the lookback window for historical demand.
	historyDays = 90
)

// ForecastingService orchestrates the full forecasting pipeline: collect
// external features, run Holt-Winters or cold-start, compute replenishment,
// record accuracy, and optionally create PO stubs.
type ForecastingService struct {
	hwEngine    *HoltWintersEngine
	coldStart   *ColdStartHandler
	replenish   *ReplenishmentCalculator
	collector   *FeatureCollector
	accuracy    *AccuracyMonitor
	resultsMu   sync.RWMutex
	results     map[string]model.ForecastResult // key = sku|hubID
	replenMu    sync.RWMutex
	replen      map[string]model.ReplenishmentResult
	seedData    map[string][]float64 // for now: in-memory demand history
}

// NewForecastingService creates the full orchestration service.
func NewForecastingService(
	hw *HoltWintersEngine,
	cs *ColdStartHandler,
	rc *ReplenishmentCalculator,
	fc *FeatureCollector,
	am *AccuracyMonitor,
) *ForecastingService {
	fs := &ForecastingService{
		hwEngine:  hw,
		coldStart: cs,
		replenish: rc,
		collector: fc,
		accuracy:  am,
		results:   make(map[string]model.ForecastResult),
		replen:    make(map[string]model.ReplenishmentResult),
		seedData:  make(map[string][]float64),
	}
	fs.seedMockData()
	return fs
}

// RunDailyForecast triggers forecasts for all SKU-hub combinations across the
// specified hubs. It uses a worker pool bounded by MaxConcurrency.
func (fs *ForecastingService) RunDailyForecast(ctx context.Context, hubIDs []string) ([]model.ForecastResponse, error) {
	// Build all pairs (for now: all seed SKUs crossed with given hubs).
	type pair struct {
		sku   string
		hubID string
	}

	var pairs []pair
	seenSKUs := make(map[string]bool)
	for sku := range fs.seedData {
		seenSKUs[sku] = true
	}
	// If no seed data, add at least one.
	if len(seenSKUs) == 0 {
		seenSKUs["FNB-001"] = true
		fs.seedData["FNB-001"] = generateMockHistory()
	}

	for sku := range seenSKUs {
		for _, hubID := range hubIDs {
			pairs = append(pairs, pair{sku, hubID})
		}
	}

	// Worker pool
	sem := make(chan struct{}, MaxConcurrency)
	var wg sync.WaitGroup
	responses := make([]model.ForecastResponse, len(pairs))
	errs := make([]error, len(pairs))

	for i, p := range pairs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, sku, hubID string) {
			defer wg.Done()
			defer func() { <-sem }()

			pairCtx, cancel := context.WithTimeout(ctx, PerPairTimeout)
			defer cancel()

			resp, err := fs.forecastSingle(pairCtx, sku, hubID)
			if err != nil {
				errs[idx] = err
				log.Printf("[forecast] error sku=%s hub=%s: %v", sku, hubID, err)
				return
			}
			responses[idx] = resp
		}(i, p.sku, p.hubID)
	}
	wg.Wait()

	// Collect non-nil errors.
	// Return first non-nil error alongside partial results.
	var firstErr error
	for _, e := range errs {
		if e != nil {
			firstErr = e
			break
		}
	}

	// Filter out empty responses.
	var result []model.ForecastResponse
	for _, r := range responses {
		if r.SKU != "" {
			result = append(result, r)
		}
	}

	return result, firstErr
}

// forecastSingle runs the full pipeline for one SKU-hub pair.
func (fs *ForecastingService) forecastSingle(ctx context.Context, sku, hubID string) (model.ForecastResponse, error) {
	// 1. Get historical demand (90-day lookback).
	history := fs.getHistory(ctx, sku, hubID)
	if len(history) == 0 {
		// TODO: pull from real inventory/order service
		history = fs.seedData[sku]
	}
	if len(history) == 0 {
		history = generateMockHistory()
	}

	// Impute stockout periods: find 48h windows, replace with mean.
	history = imputeStockouts(history)

	// 2. Collect external features.
	features := fs.collector.Collect(ctx, sku, hubID)

	// 3. Run forecast.
	var dailyForecast [7]float64
	var method string
	var confidence float64

	if len(history) >= 7 {
		params := fs.hwEngine.params(sku)
		fc := fs.hwEngine.Forecast(history, params, 7)
		for i := 0; i < 7 && i < len(fc); i++ {
			dailyForecast[i] = fc[i]
		}
		method = "holtwinters_multiplicative"
		confidence = 0.85
	} else {
		cat := model.InferSKUCategory(sku)
		catAvg := model.CategoryAverages[cat]
		catVar := model.CategoryVariances[cat]
		fc, cf, meth := fs.coldStart.Forecast(history, catAvg, catVar)
		method = meth
		confidence = cf
		for i := 0; i < 7; i++ {
			dailyForecast[i] = fc
		}
	}

	// 4. Compute daily average from forecast.
	dailyAvg := mean(dailyForecast[:])

	// 5. Compute replenishment.
	replenResult := fs.replenish.Calculate(dailyAvg, 50, nil, 3, 0.95)
	replenResult.SKU = sku
	replenResult.HubID = hubID

	// If order qty > 0, create PO stub (log for now).
	if replenResult.OrderQty > 0 {
		log.Printf("[forecast] PO stub: sku=%s hub=%s qty=%d", sku, hubID, replenResult.OrderQty)
	}

	// 6. Build forecast result.
	fcResult := model.ForecastResult{
		SKU:           sku,
		HubID:         hubID,
		DailyForecast: dailyForecast,
		Method:        method,
		Confidence:    confidence,
		GeneratedAt:   time.Now(),
		Features:      features,
	}

	// 7. Store result.
	key := sku + "|" + hubID
	fs.resultsMu.Lock()
	fs.results[key] = fcResult
	fs.replenMu.Lock()
	fs.replen[key] = replenResult
	fs.replenMu.Unlock()
	fs.resultsMu.Unlock()

	// 8. Record accuracy if we have actuals to compare.
	// For now: compare day-0 forecast against a synthetic actual.
	// TODO: pull real actuals from order service.
	if len(history) > 0 {
		actual := history[len(history)-1]
		fs.accuracy.Record(sku, hubID, dailyForecast[0], actual)
	}

	stats := fs.accuracy.GetStats(sku, hubID)

	return model.ForecastResponse{
		SKU:           sku,
		HubID:         hubID,
		Forecast:      fcResult,
		Replenishment: replenResult,
		Stats:         stats,
		Features:      features,
	}, nil
}

// GetForecast returns the latest forecast for a SKU-hub pair.
func (fs *ForecastingService) GetForecast(sku, hubID string) (model.ForecastResult, bool) {
	fs.resultsMu.RLock()
	defer fs.resultsMu.RUnlock()
	r, ok := fs.results[sku+"|"+hubID]
	return r, ok
}

// GetReplenishment returns the latest replenishment for a SKU-hub pair.
func (fs *ForecastingService) GetReplenishment(sku, hubID string) (model.ReplenishmentResult, bool) {
	fs.replenMu.RLock()
	defer fs.replenMu.RUnlock()
	r, ok := fs.replen[sku+"|"+hubID]
	return r, ok
}

// GetAccuracyStats returns MAPE stats for a SKU-hub pair.
func (fs *ForecastingService) GetAccuracyStats(sku, hubID string) model.SKUForecastStats {
	return fs.accuracy.GetStats(sku, hubID)
}

// GetDegradedSKUs returns all degraded SKU-hub pairs.
func (fs *ForecastingService) GetDegradedSKUs() []string {
	return fs.accuracy.GetDegradedSKUs()
}

// SeedExternalFeatures allows manual injection of features for testing
// (POST /admin/features).
func (fs *ForecastingService) SeedExternalFeatures(sku, hubID string, feat model.ExternalFeature) {
	fs.resultsMu.Lock()
	defer fs.resultsMu.Unlock()
	key := sku + "|" + hubID
	if r, ok := fs.results[key]; ok {
		r.Features = feat
		fs.results[key] = r
	}
}

// getHistory retrieves the last 90 days of demand data.
func (fs *ForecastingService) getHistory(_ context.Context, sku, hubID string) []float64 {
	// In production: query order/inventory service for sales data.
	// For now: return seed data.
	_ = hubID
	return fs.seedData[sku]
}

// imputeStockouts detects zero-demand periods that span approximately 48 hours
// and replaces them with the mean of the surrounding non-zero values.
func imputeStockouts(data []float64) []float64 {
	if len(data) < 3 {
		return data
	}

	out := make([]float64, len(data))
	copy(out, data)

	// Simple heuristic: consecutive zeros of length 2+ near other zeros.
	i := 0
	for i < len(out) {
		if out[i] == 0 {
			start := i
			for i < len(out) && out[i] == 0 {
				i++
			}
			end := i - 1
			span := end - start + 1

			// Impute if span is >= 2 (approx 48h with daily granularity).
			if span >= 2 {
				var surroundSum float64
				surroundCount := 0
				before := start - 1
				after := end + 1
				if before >= 0 && out[before] > 0 {
					surroundSum += out[before]
					surroundCount++
				}
				if after < len(out) && out[after] > 0 {
					surroundSum += out[after]
					surroundCount++
				}
				imputeVal := 0.0
				if surroundCount > 0 {
					imputeVal = surroundSum / float64(surroundCount)
				}
				for j := start; j <= end; j++ {
					out[j] = imputeVal
				}
			}
		} else {
			i++
		}
	}
	return out
}

// generateMockHistory creates synthetic 90-day demand data for bootstrapping.
func generateMockHistory() []float64 {
	data := make([]float64, 90)
	base := 40.0
	for i := 0; i < 90; i++ {
		// Weekly pattern: higher on weekends
		t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		wd := t.Weekday()
		mult := 1.0
		if wd == time.Saturday || wd == time.Friday {
			mult = 1.5
		}
		// Add seasonality
		season := 1.0 + 0.2*float64(i%7)/7.0
		noise := (float64(i%5) - 2) * 2.0
		v := (base + noise) * mult * season
		if v < 0 {
			v = 0
		}
		data[i] = v
	}
	return data
}

// seedMockData populates the in-memory store with sample SKUs.
func (fs *ForecastingService) seedMockData() {
	skus := []string{"FNB-001", "FRZ-002", "DAI-003", "BEV-004", "SNK-005", "GEN-006"}
	for _, sku := range skus {
		fs.seedData[sku] = generateMockHistory()
	}
}
