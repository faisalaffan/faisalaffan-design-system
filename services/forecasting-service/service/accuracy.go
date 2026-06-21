package service

import (
	"math"
	"sync"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

const (
	// WarningThreshold is the weekly MAPE above which a SKU is flagged.
	WarningThreshold = 30.0
	// CriticalThreshold is the weekly MAPE above which a SKU is critical.
	CriticalThreshold = 50.0
	// rollingWindow is the number of days tracked per SKU-hub.
	rollingWindow = 30
)

// AccuracyMonitor tracks forecast accuracy per SKU-hub pair using APE
// (Absolute Percentage Error) and computes rolling MAPE statistics.
type AccuracyMonitor struct {
	mu     sync.RWMutex
	apeMap map[string]*skuAccuracy
}

type skuAccuracy struct {
	apes [rollingWindow]float64
	pos  int // circular buffer position
	size int // number of entries stored
}

// key returns a composite key for the SKU-hub pair.
func accuracyKey(sku, hubID string) string {
	return sku + "|" + hubID
}

// NewAccuracyMonitor creates a new accuracy monitor.
func NewAccuracyMonitor() *AccuracyMonitor {
	return &AccuracyMonitor{
		apeMap: make(map[string]*skuAccuracy),
	}
}

// Record computes the APE for a single forecast-actual pair and stores it in
// the rolling 30-day window. Zero actuals are skipped (stockout / no demand).
func (am *AccuracyMonitor) Record(sku, hubID string, forecast, actual float64) {
	if actual == 0 {
		return // skip stockout or no-demand days
	}

	ape := math.Abs(forecast-actual) / actual * 100

	am.mu.Lock()
	defer am.mu.Unlock()

	key := accuracyKey(sku, hubID)
	sa, ok := am.apeMap[key]
	if !ok {
		sa = &skuAccuracy{}
		am.apeMap[key] = sa
	}

	sa.apes[sa.pos] = ape
	sa.pos = (sa.pos + 1) % rollingWindow
	if sa.size < rollingWindow {
		sa.size++
	}
}

// GetStats returns the accuracy statistics for a SKU-hub pair.
func (am *AccuracyMonitor) GetStats(sku, hubID string) model.SKUForecastStats {
	am.mu.RLock()
	defer am.mu.RUnlock()

	key := accuracyKey(sku, hubID)
	sa, ok := am.apeMap[key]
	if !ok {
		return model.SKUForecastStats{}
	}

	return computeStats(sa)
}

// GetDegradedSKUs returns all SKU-hub pairs whose weekly MAPE exceeds the
// warning threshold (30%).
func (am *AccuracyMonitor) GetDegradedSKUs() []string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var degraded []string
	for key, sa := range am.apeMap {
		stats := computeStats(sa)
		if stats.WeeklyMAPE > WarningThreshold {
			degraded = append(degraded, key)
		}
	}
	return degraded
}

// computeStats derives the daily array, weekly MAPE, monthly MAPE from a
// skuAccuracy buffer.
func computeStats(sa *skuAccuracy) model.SKUForecastStats {
	var stats model.SKUForecastStats

	// Build daily array in chronological order.
	for i := 0; i < sa.size; i++ {
		idx := (sa.pos - sa.size + i + rollingWindow) % rollingWindow
		stats.DailyMAPE[i] = sa.apes[idx]
	}
	stats.DataPoints = sa.size

	// Weekly MAPE (last 7 days).
	weekCount := 0
	weekSum := 0.0
	for i := sa.size - 1; i >= 0 && i >= sa.size-7; i-- {
		idx := (sa.pos - sa.size + i + rollingWindow) % rollingWindow
		weekSum += sa.apes[idx]
		weekCount++
	}
	if weekCount > 0 {
		stats.WeeklyMAPE = math.Round(weekSum/float64(weekCount)*100) / 100
	}

	// Monthly MAPE (all available data).
	monthSum := 0.0
	for i := 0; i < sa.size; i++ {
		idx := (sa.pos - sa.size + i + rollingWindow) % rollingWindow
		monthSum += sa.apes[idx]
	}
	if sa.size > 0 {
		stats.MonthlyMAPE = math.Round(monthSum/float64(sa.size)*100) / 100
	}

	return stats
}
