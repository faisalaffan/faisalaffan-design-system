package service

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
	"github.com/google/uuid"
)

// ReassignmentConfig controls the exponential-backoff retry behaviour.
type ReassignmentConfig struct {
	BaseBackoff time.Duration
	MaxAttempts int
}

// DefaultReassignmentConfig returns sensible reassignment defaults.
func DefaultReassignmentConfig() ReassignmentConfig {
	return ReassignmentConfig{BaseBackoff: 1 * time.Second, MaxAttempts: 3}
}

// ReassignmentHandler manages order reassignment when a driver rejects an
// assignment or fails to respond (timeout). It uses exponential backoff
// (1s, 2s, 4s) with a configurable max attempt count.
type ReassignmentHandler struct {
	matcher      *GreedyMatcher
	stateMachine *DriverStateMachine
	config       ReassignmentConfig

	mu          sync.RWMutex
	orders      map[string]*model.Order
	assignments map[string]*model.Assignment
	cancelCh    map[string]chan struct{} // orderID -> cancel channel
}

// NewReassignmentHandler creates a new ReassignmentHandler.
func NewReassignmentHandler(matcher *GreedyMatcher, sm *DriverStateMachine, cfg ReassignmentConfig) *ReassignmentHandler {
	return &ReassignmentHandler{
		matcher:      matcher,
		stateMachine: sm,
		config:       cfg,
		orders:       make(map[string]*model.Order),
		assignments:  make(map[string]*model.Assignment),
		cancelCh:     make(map[string]chan struct{}),
	}
}

// TrackOrder stores an order reference for reassignment lookups.
func (rh *ReassignmentHandler) TrackOrder(order *model.Order) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.orders[order.ID] = order
}

// TrackAssignment stores an assignment reference.
func (rh *ReassignmentHandler) TrackAssignment(a *model.Assignment) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.assignments[a.ID] = a
}

// GetAssignment retrieves a tracked assignment by ID.
func (rh *ReassignmentHandler) GetAssignment(id string) (*model.Assignment, bool) {
	rh.mu.RLock()
	defer rh.mu.RUnlock()
	a, ok := rh.assignments[id]
	return a, ok
}

// HandleReject processes a driver rejection and triggers reassignment with
// exponential backoff.
func (rh *ReassignmentHandler) HandleReject(driverID, orderID string) error {
	log.Printf("[reassign] driver %s rejected order %s", driverID, orderID)

	if err := rh.stateMachine.Transition(driverID, model.DriverIdle, "rejected assignment"); err != nil {
		return fmt.Errorf("reject transition: %w", err)
	}

	rh.mu.RLock()
	order, ok := rh.orders[orderID]
	rh.mu.RUnlock()
	if !ok {
		return fmt.Errorf("order %s not tracked", orderID)
	}

	go rh.retry(order, 1)
	return nil
}

// HandleTimeout processes an assignment timeout and triggers reassignment.
func (rh *ReassignmentHandler) HandleTimeout(driverID, orderID string) error {
	log.Printf("[reassign] driver %s timed out on order %s", driverID, orderID)

	if err := rh.stateMachine.Transition(driverID, model.DriverIdle, "assignment timeout"); err != nil {
		return fmt.Errorf("timeout transition: %w", err)
	}

	rh.mu.RLock()
	order, ok := rh.orders[orderID]
	rh.mu.RUnlock()
	if !ok {
		return fmt.Errorf("order %s not tracked", orderID)
	}

	go rh.retry(order, 1)
	return nil
}

// CancelRetry cancels an active retry loop for an order.
func (rh *ReassignmentHandler) CancelRetry(orderID string) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	if ch, ok := rh.cancelCh[orderID]; ok {
		close(ch)
		delete(rh.cancelCh, orderID)
	}
}

// retry attempts to find a replacement driver using exponential backoff.
func (rh *ReassignmentHandler) retry(order *model.Order, attempt int) {
	if attempt > rh.config.MaxAttempts {
		log.Printf("[reassign] max attempts (%d) reached for order %s", rh.config.MaxAttempts, order.ID)
		order.Status = model.OrderFailed
		return
	}

	backoff := rh.config.BaseBackoff * time.Duration(math.Pow(2, float64(attempt-1)))
	log.Printf("[reassign] order %s attempt %d/%d, backoff %v", order.ID, attempt, rh.config.MaxAttempts, backoff)

	select {
	case <-time.After(backoff):
	case <-rh.getOrCreateCancel(order.ID):
		return
	}

	drivers := rh.stateMachine.AvailableDrivers()
	best, score := rh.matcher.FindBestDriver(order, drivers)
	if best == nil {
		log.Printf("[reassign] no available drivers for order %s, retrying", order.ID)
		rh.retry(order, attempt+1)
		return
	}

	if err := rh.stateMachine.Transition(best.ID, model.DriverAssigned, "reassignment"); err != nil {
		log.Printf("[reassign] transition for driver %s failed: %v", best.ID, err)
		rh.retry(order, attempt+1)
		return
	}

	order.Status = model.OrderAssigned
	order.AssignedDriverID = best.ID
	best.Load++
	best.SkippedCount = 0

	asgn := &model.Assignment{
		ID:        uuid.New().String(),
		OrderID:   order.ID,
		DriverID:  best.ID,
		Status:    "PENDING",
		Score:     score,
		Attempt:   attempt + 1,
		CreatedAt: time.Now(),
	}
	rh.TrackAssignment(asgn)

	log.Printf("[reassign] order %s reassigned to driver %s (attempt %d, score=%.4f)",
		order.ID, best.ID, attempt+1, score)
}

func (rh *ReassignmentHandler) getOrCreateCancel(orderID string) chan struct{} {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	if ch, ok := rh.cancelCh[orderID]; ok {
		return ch
	}
	ch := make(chan struct{})
	rh.cancelCh[orderID] = ch
	return ch
}
