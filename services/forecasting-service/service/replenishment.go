package service

import (
	"math"
	"sort"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

// ReplenishmentCalculator computes reorder points, safety stock levels, and
// order quantities using FEFO (First Expiry, First Out) batch prioritisation.
type ReplenishmentCalculator struct{}

// NewReplenishmentCalculator creates a new calculator.
func NewReplenishmentCalculator() *ReplenishmentCalculator {
	return &ReplenishmentCalculator{}
}

const (
	minOrderQty = 1
	maxOrderQty = 500
)

// zFactor returns the z-score for a given service level.
func zFactor(serviceLevel float64) float64 {
	if serviceLevel >= 0.99 {
		return 2.33
	}
	if serviceLevel >= 0.95 {
		return 1.65
	}
	if serviceLevel >= 0.90 {
		return 1.28
	}
	return 1.65 // default to 95%
}

// Calculate computes the full replenishment result for a SKU-hub pair.
func (rc *ReplenishmentCalculator) Calculate(
	forecastDaily float64,
	onHand int,
	incoming []model.InventoryBatch,
	leadTimeDays int,
	serviceLevel float64,
) model.ReplenishmentResult {
	if forecastDaily <= 0 {
		forecastDaily = 1.0 // prevent division by zero
	}
	if leadTimeDays <= 0 {
		leadTimeDays = 1
	}
	if serviceLevel <= 0 {
		serviceLevel = 0.95
	}

	// sigma estimate: assume CV = 0.3 (demand variability proxy)
	sigma := forecastDaily * 0.3
	z := zFactor(serviceLevel)

	safetyStock := z * sigma * math.Sqrt(float64(leadTimeDays))
	reorderPoint := forecastDaily*float64(leadTimeDays) + safetyStock

	// Total incoming stock
	totalIncoming := 0
	for _, b := range incoming {
		totalIncoming += b.Quantity
	}

	netPosition := float64(onHand) + float64(totalIncoming)
	orderQty := reorderPoint - netPosition + minOrderQty
	if orderQty < minOrderQty {
		orderQty = minOrderQty
	}
	if orderQty > maxOrderQty {
		orderQty = maxOrderQty
	}

	daysOfCover := (float64(onHand) + float64(totalIncoming)) / forecastDaily

	// Priority classification
	priority := "normal"
	switch {
	case daysOfCover < 1:
		priority = "high"
	case daysOfCover > 5:
		priority = "low"
	}

	// FEFO sort: earliest expiry first
	fefoBatches := make([]model.InventoryBatch, len(incoming))
	copy(fefoBatches, incoming)
	sort.Slice(fefoBatches, func(i, j int) bool {
		return fefoBatches[i].ExpiryDate.Before(fefoBatches[j].ExpiryDate)
	})

	return model.ReplenishmentResult{
		SKU:          "",
		HubID:        "",
		ReorderPoint: math.Round(reorderPoint*100) / 100,
		SafetyStock:  math.Round(safetyStock*100) / 100,
		OrderQty:     int(math.Round(orderQty)),
		DaysOfCover:  math.Round(daysOfCover*100) / 100,
		Priority:     priority,
		FEFOBatches:  fefoBatches,
	}
}
