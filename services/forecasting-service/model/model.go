package model

import "time"

// TimeSeriesPoint represents a single data point in a time series.
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Imputed   bool      `json:"imputed"`
}

// ForecastResult is the output of a forecast run for a single SKU-hub pair.
type ForecastResult struct {
	SKU           string         `json:"sku"`
	HubID         string         `json:"hub_id"`
	DailyForecast [7]float64     `json:"daily_forecast"`
	Method        string         `json:"method"`
	Confidence    float64        `json:"confidence"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Features      ExternalFeature `json:"features,omitempty"`
}

// ExternalFeature holds contextual signals that influence demand.
type ExternalFeature struct {
	Temperature    float64 `json:"temperature"`
	Humidity       float64 `json:"humidity"`
	IsRaining      bool    `json:"is_raining"`
	IsWeekend      bool    `json:"is_weekend"`
	IsPublicHoliday bool   `json:"is_public_holiday"`
	HasPromo       bool    `json:"has_promo"`
	PromoDiscountPct float64 `json:"promo_discount_pct"`
	Season         string  `json:"season"`
	RamadhanMode   bool    `json:"ramadhan_mode"`
}

// HoltWintersParams holds the smoothing parameters and season length.
type HoltWintersParams struct {
	Alpha        float64 `json:"alpha"`
	Beta         float64 `json:"beta"`
	Gamma        float64 `json:"gamma"`
	SeasonLength int     `json:"season_length"`
}

// DefaultHoltWintersParams returns sensible defaults.
func DefaultHoltWintersParams() HoltWintersParams {
	return HoltWintersParams{
		Alpha:        0.3,
		Beta:         0.1,
		Gamma:        0.2,
		SeasonLength: 7,
	}
}

// InventoryBatch represents a single incoming inventory batch with expiry.
type InventoryBatch struct {
	BatchID    string    `json:"batch_id"`
	SKU        string    `json:"sku"`
	Quantity   int       `json:"quantity"`
	ExpiryDate time.Time `json:"expiry_date"`
}

// ReplenishmentResult is the output of replenishment calculation.
type ReplenishmentResult struct {
	SKU           string           `json:"sku"`
	HubID         string           `json:"hub_id"`
	ReorderPoint  float64          `json:"reorder_point"`
	SafetyStock   float64          `json:"safety_stock"`
	OrderQty      int              `json:"order_qty"`
	DaysOfCover   float64          `json:"days_of_cover"`
	Priority      string           `json:"priority"`
	FEFOBatches   []InventoryBatch `json:"fefo_batches,omitempty"`
}

// SKUForecastStats holds accuracy statistics for a SKU-hub pair.
type SKUForecastStats struct {
	DailyMAPE    [30]float64 `json:"daily_mape"`
	WeeklyMAPE   float64     `json:"weekly_mape"`
	MonthlyMAPE  float64     `json:"monthly_mape"`
	DataPoints   int         `json:"data_points"`
}

// StockoutPeriod represents a detected stockout event.
type StockoutPeriod struct {
	SKU     string    `json:"sku"`
	HubID   string    `json:"hub_id"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
}

// ForecastRequest is the request body for POST /forecast/run.
type ForecastRequest struct {
	HubIDs []string `json:"hub_ids" binding:"required"`
}

// ForecastResponse is the response body for forecast endpoints.
type ForecastResponse struct {
	SKU             string             `json:"sku"`
	HubID           string             `json:"hub_id"`
	Forecast        ForecastResult     `json:"forecast"`
	Replenishment   ReplenishmentResult `json:"replenishment"`
	Stats           SKUForecastStats   `json:"stats,omitempty"`
	Features        ExternalFeature    `json:"features"`
}

// InferSKUCategory returns a category string based on SKU prefix.
func InferSKUCategory(sku string) string {
	if len(sku) < 3 {
		return "general"
	}
	prefix := sku[:3]
	switch prefix {
	case "FNB":
		return "food"
	case "FRZ":
		return "frozen"
	case "DAI":
		return "dairy"
	case "BEV":
		return "beverage"
	case "SNK":
		return "snack"
	default:
		return "general"
	}
}

// CategoryAverages holds baseline demand averages per category.
var CategoryAverages = map[string]float64{
	"food":     45.0,
	"frozen":   18.0,
	"dairy":    60.0,
	"beverage": 35.0,
	"snack":    25.0,
	"general":  20.0,
}

// CategoryVariances holds baseline demand variances per category.
var CategoryVariances = map[string]float64{
	"food":     200.0,
	"frozen":   80.0,
	"dairy":    150.0,
	"beverage": 120.0,
	"snack":    90.0,
	"general":  100.0,
}
