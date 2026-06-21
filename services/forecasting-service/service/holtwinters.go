package service

import (
	"math"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

// HoltWintersEngine implements triple exponential smoothing using the
// multiplicative seasonality formulation.
type HoltWintersEngine struct {
	perSKUParams map[string]model.HoltWintersParams
}

// NewHoltWintersEngine creates a new engine with optional per-SKU parameter overrides.
func NewHoltWintersEngine() *HoltWintersEngine {
	return &HoltWintersEngine{
		perSKUParams: make(map[string]model.HoltWintersParams),
	}
}

// SetParams overrides the default parameters for a specific SKU.
func (hw *HoltWintersEngine) SetParams(sku string, p model.HoltWintersParams) {
	hw.perSKUParams[sku] = p
}

// params returns the parameters to use for the given SKU.
func (hw *HoltWintersEngine) params(sku string) model.HoltWintersParams {
	if p, ok := hw.perSKUParams[sku]; ok {
		return p
	}
	return model.DefaultHoltWintersParams()
}

// Forecast runs multiplicative Holt-Winters on historical data and returns
// `steps` forecasts ahead.
func (hw *HoltWintersEngine) Forecast(historical []float64, params model.HoltWintersParams, steps int) []float64 {
	n := len(historical)
	if n == 0 {
		return make([]float64, steps)
	}

	m := params.SeasonLength
	if m <= 0 {
		m = 7
	}

	// If not enough data for one full season, return mean-based forecast.
	if n < m {
		mean := mean(historical)
		fc := make([]float64, steps)
		for i := range fc {
			fc[i] = mean
		}
		return fc
	}

	// ---- Initialize ----
	// Level: average of first season
	level := mean(historical[:m])

	// Trend: average of seasonal differences
	var trend float64
	diffs := 0
	for i := m; i < min(m*2, n); i++ {
		trend += (historical[i] - historical[i-m]) / float64(m)
		diffs++
	}
	if diffs > 0 {
		trend /= float64(diffs)
	}
	if trend == 0 {
		trend = 0.01 // avoid flat trend
	}

	// Seasonalities: data[i]/level for first season
	seasonalities := make([]float64, m)
	for i := 0; i < m; i++ {
		if level > 0 {
			seasonalities[i] = historical[i] / level
		} else {
			seasonalities[i] = 1.0
		}
		if seasonalities[i] <= 0 {
			seasonalities[i] = 0.1
		}
	}

	alpha := clamp(params.Alpha, 0.01, 0.99)
	beta := clamp(params.Beta, 0.01, 0.99)
	gamma := clamp(params.Gamma, 0.01, 0.99)

	// ---- Smoothing pass ----
	smoothed := make([]float64, n)
	for i := 0; i < n; i++ {
		oldLevel := level
		sIdx := i % m

		// Multiplicative update formulas
		if historical[i] != 0 && seasonalities[sIdx] > 0 {
			level = alpha*(historical[i]/seasonalities[sIdx]) + (1-alpha)*(level+trend)
		} else {
			level = level + trend
		}
		if level <= 0 {
			level = 0.01
		}

		trend = beta*(level-oldLevel) + (1-beta)*trend
		if trend <= 0 {
			trend = 0.01
		}

		seasonalities[sIdx] = gamma*(historical[i]/level) + (1-gamma)*seasonalities[sIdx]
		if seasonalities[sIdx] <= 0 {
			seasonalities[sIdx] = 0.1
		}

		smoothed[i] = level + trend // one-step ahead fitted
	}

	// ---- Forecast future steps ----
	forecasts := make([]float64, steps)
	lastIdx := n - 1
	fLevel := level
	fTrend := trend
	for i := 0; i < steps; i++ {
		sIdx := (lastIdx + 1 + i) % m
		val := (fLevel + fTrend*float64(i+1)) * seasonalities[sIdx]
		if val < 0 {
			val = 0
		}
		forecasts[i] = val
	}

	return forecasts
}

// TrackMAPE computes the Mean Absolute Percentage Error between actuals and
// the one-step-ahead smoothed values produced during Holt-Winters fitting.
// It returns the APE for each step (absolute percentage error).
func (hw *HoltWintersEngine) TrackMAPE(historical []float64, params model.HoltWintersParams) []float64 {
	n := len(historical)
	if n < 2 {
		return nil
	}

	m := params.SeasonLength
	if m <= 0 {
		m = 7
	}

	// Same initialization as Forecast
	level := mean(historical[:min(m, n)])

	var trend float64
	diffs := 0
	for i := m; i < min(m*2, n); i++ {
		trend += (historical[i] - historical[i-m]) / float64(m)
		diffs++
	}
	if diffs > 0 {
		trend /= float64(diffs)
	}
	if trend == 0 {
		trend = 0.01
	}

	seasonalities := make([]float64, m)
	for i := 0; i < min(m, n); i++ {
		if level > 0 {
			seasonalities[i] = historical[i] / level
		} else {
			seasonalities[i] = 1.0
		}
		if seasonalities[i] <= 0 {
			seasonalities[i] = 0.1
		}
	}

	alpha := clamp(params.Alpha, 0.01, 0.99)
	beta := clamp(params.Beta, 0.01, 0.99)
	gamma := clamp(params.Gamma, 0.01, 0.99)

	apes := make([]float64, 0, n)

	for i := 0; i < n; i++ {
		oldLevel := level
		sIdx := i % m

		// Compute forecast before updating (this is the "prediction" for this point)
		var predicted float64
		if i > 0 {
			predicted = (oldLevel + trend) * seasonalities[sIdx]
		} else {
			predicted = historical[i]
		}

		// Update
		if historical[i] != 0 && seasonalities[sIdx] > 0 {
			level = alpha*(historical[i]/seasonalities[sIdx]) + (1-alpha)*(level+trend)
		} else {
			level = level + trend
		}
		if level <= 0 {
			level = 0.01
		}
		trend = beta*(level-oldLevel) + (1-beta)*trend
		if trend <= 0 {
			trend = 0.01
		}
		seasonalities[sIdx] = gamma*(historical[i]/level) + (1-gamma)*seasonalities[sIdx]
		if seasonalities[sIdx] <= 0 {
			seasonalities[sIdx] = 0.1
		}

		// Track APE for actual data points (skip zero actuals)
		if historical[i] > 0 && predicted > 0 {
			ape := math.Abs(historical[i]-predicted) / historical[i] * 100
			apes = append(apes, ape)
		}
	}

	return apes
}

// mean computes the arithmetic mean. Returns 0 for empty slice.
func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
