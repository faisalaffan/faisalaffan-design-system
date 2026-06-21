package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/forecasting-service/model"
)

// FeatureCollector gathers external signals (weather, holidays, promotions)
// that influence demand. In production these would call real APIs; this
// implementation provides mock data as a placeholder.
type FeatureCollector struct {
	holidays map[string]bool
}

// NewFeatureCollector creates a collector seeded with known public holidays.
func NewFeatureCollector() *FeatureCollector {
	return &FeatureCollector{
		holidays: seedHolidays(),
	}
}

// SetHolidays replaces the holiday map (useful for testing).
func (fc *FeatureCollector) SetHolidays(h map[string]bool) {
	fc.holidays = h
}

// Collect gathers all external features concurrently with a 10-second timeout.
// Failures log a warning and return zero values -- they never block the caller.
func (fc *FeatureCollector) Collect(ctx context.Context, sku, hubID string) model.ExternalFeature {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	type result struct {
		temp     float64
		humidity float64
		raining  bool
	}

	weatherCh := make(chan result, 1)
	holidayCh := make(chan bool, 1)
	promoCh := make(chan struct {
		has  bool
		pct  float64
	}, 1)

	// Weather goroutine
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			temp, hum, rain := fc.fetchWeather(hubID)
			weatherCh <- result{temp, hum, rain}
		}
	}()

	// Holiday goroutine
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			holidayCh <- fc.isHoliday(time.Now())
		}
	}()

	// Promo goroutine
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			has, pct := fc.fetchPromo(sku)
			promoCh <- struct {
				has  bool
				pct  float64
			}{has, pct}
		}
	}()

	now := time.Now()
	isWeekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
	season := inferSeason(now)
	ramadhan := isRamadhan(now)

	var weather result
	select {
	case w := <-weatherCh:
		weather = w
	case <-ctx.Done():
		log.Printf("[collector] weather fetch timed out for hub=%s", hubID)
	}

	isHoliday := false
	select {
	case h := <-holidayCh:
		isHoliday = h
	case <-ctx.Done():
	}

	hasPromo := false
	var promoPct float64
	select {
	case p := <-promoCh:
		hasPromo = p.has
		promoPct = p.pct
	case <-ctx.Done():
	}

	return model.ExternalFeature{
		Temperature:      weather.temp,
		Humidity:         weather.humidity,
		IsRaining:        weather.raining,
		IsWeekend:        isWeekend,
		IsPublicHoliday:  isHoliday,
		HasPromo:         hasPromo,
		PromoDiscountPct: promoPct,
		Season:           season,
		RamadhanMode:     ramadhan,
	}
}

// fetchWeather is a placeholder. Replace with real OpenWeatherMap / BMKG API.
func (fc *FeatureCollector) fetchWeather(hubID string) (temp, humidity float64, raining bool) {
	// In production: call external weather API.
	// Mock: hubID suffix hints at conditions.
	id := strings.ToLower(hubID)
	switch {
	case strings.Contains(id, "jkt") || strings.Contains(id, "jakarta"):
		return 29.0, 78.0, true
	case strings.Contains(id, "bdo") || strings.Contains(id, "bandung"):
		return 22.0, 80.0, true
	case strings.Contains(id, "sby") || strings.Contains(id, "surabaya"):
		return 32.0, 65.0, false
	case strings.Contains(id, "dps") || strings.Contains(id, "bali"):
		return 28.0, 70.0, false
	default:
		return 27.0, 72.0, false
	}
}

// isHoliday checks whether the given date is a known public holiday.
func (fc *FeatureCollector) isHoliday(t time.Time) bool {
	key := t.Format("2006-01-02")
	return fc.holidays[key]
}

// fetchPromo is a placeholder. Replace with real promo engine client.
func (fc *FeatureCollector) fetchPromo(sku string) (bool, float64) {
	// In production: query promo-engine service.
	_ = sku
	return false, 0
}

// inferSeason returns the Indonesian season based on month.
func inferSeason(t time.Time) string {
	m := t.Month()
	switch {
	case m >= 11 || m <= 3:
		return "rainy"
	case m >= 4 && m <= 10:
		return "dry"
	default:
		return "transition"
	}
}

// isRamadhan returns true if the date falls within Ramadhan 1447 H (approximate).
// Estimated: March 20, 2026 - April 18, 2026.
func isRamadhan(t time.Time) bool {
	start := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 18, 23, 59, 59, 0, time.UTC)
	return !t.Before(start) && !t.After(end)
}

// seedHolidays returns a set of known Indonesian public holidays for 2026.
func seedHolidays() map[string]bool {
	return map[string]bool{
		"2026-01-01": true, // Tahun Baru
		"2026-03-21": true, // Isra Mikraj (approx)
		"2026-03-31": true, // Hari Raya Nyepi
		"2026-04-10": true, // Wafat Isa Almasih
		"2026-04-17": true, // Idul Fitri day 1 (approx)
		"2026-04-18": true, // Idul Fitri day 2 (approx)
		"2026-05-01": true, // Hari Buruh
		"2026-05-21": true, // Kenaikan Yesus Kristus
		"2026-06-01": true, // Hari Lahir Pancasila
		"2026-06-24": true, // Idul Adha (approx)
		"2026-08-17": true, // Hari Kemerdekaan
		"2026-12-25": true, // Hari Natal
	}
}
