package circuitbreaker

import (
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

// CircuitBreaker implements a simple circuit breaker pattern.
// When consecutive failures exceed the threshold, the circuit opens.
// After a reset timeout, it transitions to half-open, allowing one probe request.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        State
	failures     int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
	name         string
}

// New creates a new CircuitBreaker.
func New(name string, threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:         name,
		state:        Closed,
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// Execute wraps a function with circuit breaker protection.
// If the circuit is open, the function is not called and an error is returned.
// If the circuit is half-open and the function succeeds, the circuit closes.
// If the function fails, the failure count increments and may open the circuit.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == Open {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = HalfOpen
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit %s is OPEN", cb.name)
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()
		if cb.failures >= cb.threshold {
			cb.state = Open
		}
		return err
	}
	cb.failures = 0
	if cb.state == HalfOpen {
		cb.state = Closed
	}
	return nil
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Name returns the circuit breaker name.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Counts returns the current failure count and threshold.
func (cb *CircuitBreaker) Counts() (failures, threshold int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures, cb.threshold
}
