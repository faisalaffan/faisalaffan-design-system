package middleware

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Histogram tracks observation distribution across configurable buckets.
type Histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

// NewHistogram creates a histogram with the given bucket boundaries (in ms).
func NewHistogram(buckets []float64) *Histogram {
	return &Histogram{buckets: buckets, counts: make([]int64, len(buckets))}
}

// Observe records a value in the histogram.
func (h *Histogram) Observe(val float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += val
	h.count++
	for i, b := range h.buckets {
		if val <= b {
			h.counts[i]++
			break
		}
	}
}

// MetricsCollector provides Prometheus-style counters and histograms
// using only stdlib primitives — no external dependency.
type MetricsCollector struct {
	mu         sync.RWMutex
	counters   map[string]*atomic.Int64
	histograms map[string]*Histogram
}

// GlobalMetrics is the default application-wide metrics collector.
var GlobalMetrics = &MetricsCollector{
	counters:   make(map[string]*atomic.Int64),
	histograms: make(map[string]*Histogram),
}

// GetMetrics returns the global metrics collector.
func GetMetrics() *MetricsCollector { return GlobalMetrics }

// Inc increments a named counter, creating it lazily.
func (m *MetricsCollector) Inc(name string) {
	m.mu.RLock()
	c, ok := m.counters[name]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		c = &atomic.Int64{}
		m.counters[name] = c
		m.mu.Unlock()
	}
	c.Add(1)
}

// Get returns the current value of a named counter.
func (m *MetricsCollector) Get(name string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.counters[name]; ok {
		return c.Load()
	}
	return 0
}

// ObserveHistogram records a value in a named histogram, creating it lazily.
func (m *MetricsCollector) ObserveHistogram(name string, val float64) {
	m.mu.RLock()
	h, ok := m.histograms[name]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		h = NewHistogram([]float64{1, 5, 10, 50, 100, 500, 1000, 5000})
		m.histograms[name] = h
		m.mu.Unlock()
	}
	h.Observe(val)
}

// Snapshot returns a serializable snapshot of all metrics.
func (m *MetricsCollector) Snapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := map[string]interface{}{}
	for k, v := range m.counters {
		result[k] = v.Load()
	}
	for k, h := range m.histograms {
		h.mu.Lock()
		result[fmt.Sprintf("%s_avg_ms", k)] = h.sum / float64(max(h.count, 1))
		result[fmt.Sprintf("%s_count", k)] = h.count
		h.mu.Unlock()
	}
	return result
}

// Metrics returns a Gin middleware that tracks request duration and status codes.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := float64(time.Since(start).Microseconds()) / 1000.0
		GlobalMetrics.ObserveHistogram("http_request_duration_ms", dur)
		GlobalMetrics.Inc(fmt.Sprintf("http_%s_%d", c.Request.Method, c.Writer.Status()))
	}
}
