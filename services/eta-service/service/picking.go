package service

import (
	"math/rand"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/eta-service/model"
)

var (
	pRng   = rand.New(rand.NewSource(time.Now().UnixNano()))
	pRngMu sync.Mutex
)

func pNormFloat64() float64 {
	pRngMu.Lock()
	defer pRngMu.Unlock()
	return pRng.NormFloat64()
}

// PickingEstimator computes picking time based on item count and hub load.
type PickingEstimator struct {
	config model.ETAConfig
}

// NewPickingEstimator creates a new PickingEstimator.
func NewPickingEstimator(cfg model.ETAConfig) *PickingEstimator {
	return &PickingEstimator{config: cfg}
}

// Estimate returns picking time in seconds.
// picking_time = hub_overhead + N × per_item_mean + noise × load_multiplier
// load_multiplier = 1.0 if load ≤ 1.0, else 1.0 + (load-1.0) × 0.3
func (e *PickingEstimator) Estimate(req *model.ETARequest, load float64) float64 {
	noise := pNormFloat64() * e.config.PickingPerItemStd
	loadMult := 1.0
	if load > 1.0 {
		loadMult = 1.0 + (load-1.0)*0.3
	}
	t := e.config.HubOverheadFixed + float64(req.ItemCount)*e.config.PickingPerItemMean + noise*loadMult
	if t < e.config.HubOverheadFixed {
		return e.config.HubOverheadFixed
	}
	return t
}
