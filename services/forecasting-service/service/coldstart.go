package service

import (
	"math"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

// ColdStartHandler provides forecasts for SKU-hub pairs with little or no
// historical demand data, using Bayesian shrinkage toward a category average.
type ColdStartHandler struct {
	globalDefault float64
}

// NewColdStartHandler creates a handler with a global default fallback.
func NewColdStartHandler() *ColdStartHandler {
	return &ColdStartHandler{
		globalDefault: model.CategoryAverages["general"],
	}
}

// Forecast produces a demand estimate for a SKU with sparse history.
//
//   - 0 data points       -> pure category average, confidence 0.3
//   - 1-6 data points     -> Bayesian shrinkage, confidence 0.3..0.9
//   - No category average -> global default, confidence 0.2
func (cs *ColdStartHandler) Forecast(data []float64, categoryAvg float64, categoryVariance float64) (forecast float64, confidence float64, method string) {
	n := len(data)

	// No data at all: return category average or global default.
	if n == 0 {
		if categoryAvg > 0 {
			return categoryAvg, 0.3, "category_average"
		}
		return cs.globalDefault, 0.2, "global_default"
	}

	// Enough data for Holt-Winters: delegate upward.
	if n >= 7 {
		hw := NewHoltWintersEngine()
		// Return single-step forecast (day 1) as the point estimate.
		fc := hw.Forecast(data, model.DefaultHoltWintersParams(), 1)
		if len(fc) > 0 {
			return fc[0], 0.85, "holtwinters"
		}
	}

	// 1-6 data points: Bayesian shrinkage.
	skuMean := mean(data)
	if categoryAvg <= 0 {
		// Fallback to global default if no category info.
		categoryAvg = cs.globalDefault
	}

	// Determine prior strength based on variance.
	priorStrength := 5.0
	if categoryVariance > 100 {
		priorStrength = 3.0
	} else if categoryVariance < 10 {
		priorStrength = 10.0
	}

	// Weight = n / (n + priorStrength)
	weight := float64(n) / (float64(n) + priorStrength)
	forecast = weight*skuMean + (1-weight)*categoryAvg
	if forecast < 0 {
		forecast = 0
	}

	confidence = math.Min(0.3+float64(n)*0.1, 0.9)
	method = "bayesian_shrinkage"
	return
}
