package service

import (
	"math"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/pricing-service/model"
)

// SurgeDetector applies cascading thresholds to load ratio and smooths
// the multiplier via exponential decay when load is dropping.
type SurgeDetector struct {
	config       model.SurgeConfig
	decayMinutes float64
	lastLoad     map[string]lastLoadEntry
	mu           sync.RWMutex
}

type lastLoadEntry struct {
	loadRatio float64
	lastMult  float64
	timestamp time.Time
}

// NewSurgeDetector creates a new SurgeDetector.
// decayMinutes is the time window over which the multiplier decays to the new level.
func NewSurgeDetector(cfg model.SurgeConfig, decayMinutes float64) *SurgeDetector {
	if decayMinutes <= 0 {
		decayMinutes = 15
	}
	return &SurgeDetector{
		config:       cfg,
		decayMinutes: decayMinutes,
		lastLoad:     make(map[string]lastLoadEntry),
	}
}

// Detect returns the surge multiplier for the given area and load ratio.
// The multiplier is smoothed via exponential decay when load decreases.
// Result is capped at 3.0x.
func (d *SurgeDetector) Detect(areaID string, loadRatio float64) float64 {
	rawMult := d.cascadeMultiplier(loadRatio)

	d.mu.RLock()
	last, exists := d.lastLoad[areaID]
	d.mu.RUnlock()

	adjusted := rawMult
	if exists && loadRatio < last.loadRatio {
		// Exponential decay toward the new multiplier
		elapsed := time.Since(last.timestamp).Minutes()
		decayFactor := 1.0 - (elapsed / d.decayMinutes)
		decayFactor = max(decayFactor, 0)
		adjusted = last.lastMult + (rawMult-last.lastMult)*decayFactor
	}

	adjusted = math.Min(adjusted, 3.0)

	d.mu.Lock()
	d.lastLoad[areaID] = lastLoadEntry{
		loadRatio: loadRatio,
		lastMult:  adjusted,
		timestamp: time.Now(),
	}
	d.mu.Unlock()

	return adjusted
}

// cascadeMultiplier applies the cascading threshold logic:
//
//	< 1.0  → 1.0x (no surge)
//	1.0–2.0 → 1.2x
//	2.0–3.5 → 1.5x
//	3.5–5.0 → 2.0x
//	>= 5.0  → 3.0x
func (d *SurgeDetector) cascadeMultiplier(loadRatio float64) float64 {
	switch {
	case loadRatio >= d.config.CriticalThreshold:
		return d.config.CriticalMultiplier
	case loadRatio >= d.config.HighThreshold:
		return d.config.HighMultiplier
	case loadRatio >= d.config.MediumThreshold:
		return d.config.MediumMultiplier
	case loadRatio >= d.config.LowThreshold:
		return d.config.LowMultiplier
	default:
		return 1.0
	}
}
