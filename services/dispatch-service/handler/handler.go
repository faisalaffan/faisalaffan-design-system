package handler

import (
	"log"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/pkg/kit"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DispatchHandler exposes the dispatch-service HTTP endpoints.
type DispatchHandler struct {
	batcher      *service.BatchCollector
	matcher      *service.GreedyMatcher
	stateMachine *service.DriverStateMachine
	reassign     *service.ReassignmentHandler

	mu          sync.RWMutex
	orders      map[string]*model.Order
	assignments map[string]*model.Assignment
}

// NewDispatchHandler creates the handler and starts the batch processing
// background loop.
func NewDispatchHandler(
	batcher *service.BatchCollector,
	matcher *service.GreedyMatcher,
	sm *service.DriverStateMachine,
	re *service.ReassignmentHandler,
) *DispatchHandler {
	h := &DispatchHandler{
		batcher:      batcher,
		matcher:      matcher,
		stateMachine: sm,
		reassign:     re,
		orders:       make(map[string]*model.Order),
		assignments:  make(map[string]*model.Assignment),
	}

	batcher.Start()
	go h.processBatches()

	return h
}

// processBatches is the background loop that consumes batches from the
// collector, matches them against available drivers, and records assignments.
func (h *DispatchHandler) processBatches() {
	for batch := range h.batcher.Batches() {
		drivers := h.stateMachine.AvailableDrivers()
		if len(drivers) == 0 {
			log.Printf("[dispatch] batch of %d orders skipped — no available drivers", len(batch))
			continue
		}

		assignments := h.matcher.Match(batch, drivers)

		h.mu.Lock()
		for i := range assignments {
			a := &assignments[i]
			h.assignments[a.ID] = a
			if order, ok := h.orders[a.OrderID]; ok {
				order.Status = model.OrderAssigned
				order.AssignedDriverID = a.DriverID
			}
			h.reassign.TrackOrder(h.orders[a.OrderID])
			h.reassign.TrackAssignment(a)
		}
		h.mu.Unlock()

		log.Printf("[dispatch] batch of %d orders matched — %d assignments created",
			len(batch), len(assignments))
	}
}

// CreateOrder handles POST /dispatch/order.
func (h *DispatchHandler) CreateOrder(c *gin.Context) {
	var req model.DispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "invalid request: customer_id, items, delivery_location, hub_id required")
		return
	}

	order := &model.Order{
		ID:               uuid.New().String(),
		CustomerID:       req.CustomerID,
		Items:            req.Items,
		Total:            req.Total,
		Status:           model.OrderPending,
		Priority:         req.Priority,
		CreatedAt:        time.Now(),
		DeliveryLocation: *req.DeliveryLocation,
		HubID:            req.HubID,
	}

	if order.Priority < 1 {
		order.Priority = 1
	}

	h.mu.Lock()
	h.orders[order.ID] = order
	h.mu.Unlock()

	h.reassign.TrackOrder(order)
	h.batcher.Orders() <- order

	kit.Created(c, gin.H{
		"order_id": order.ID,
		"status":   order.Status,
	})
}

// UpdateDriverLocation handles POST /dispatch/driver/location.
func (h *DispatchHandler) UpdateDriverLocation(c *gin.Context) {
	var req model.LocationUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "driver_id, lat, lng required")
		return
	}

	if err := h.stateMachine.UpdateLocation(req.DriverID, req.Lat, req.Lng); err != nil {
		kit.BadRequest(c, err.Error())
		return
	}
	kit.OK(c, gin.H{"updated": true})
}

// AcceptAssignment handles POST /dispatch/driver/accept.
func (h *DispatchHandler) AcceptAssignment(c *gin.Context) {
	var req model.AcceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "assignment_id and driver_id required")
		return
	}

	// Validate driver
	driver, ok := h.stateMachine.GetDriver(req.DriverID)
	if !ok {
		kit.BadRequest(c, "driver not found")
		return
	}

	// Validate assignment
	h.mu.RLock()
	a, ok := h.assignments[req.AssignmentID]
	h.mu.RUnlock()
	if !ok {
		kit.BadRequest(c, "assignment not found")
		return
	}
	if a.DriverID != req.DriverID {
		kit.BadRequest(c, "assignment does not belong to this driver")
		return
	}
	if a.Status != "PENDING" {
		kit.BadRequest(c, "assignment is not pending")
		return
	}

	// Mark assignment as accepted
	now := time.Now()
	a.Status = "ACCEPTED"
	a.AcceptedAt = &now

	// Transition driver ASSIGNED -> TO_HUB
	// Transition driver from ASSIGNED (or IDLE) to TO_HUB
	if driver.Status == model.DriverIdle {
		if err := h.stateMachine.Transition(driver.ID, model.DriverAssigned, "pre-accept assignment"); err != nil {
			kit.InternalError(c, "state transition failed: "+err.Error())
			return
		}
	}
	if err := h.stateMachine.Transition(driver.ID, model.DriverToHub, "driver accepted assignment"); err != nil {
		kit.InternalError(c, "state transition failed: "+err.Error())
		return
	}

	// Update order status
	h.mu.RLock()
	order, orderOk := h.orders[a.OrderID]
	h.mu.RUnlock()
	if orderOk {
		order.Status = model.OrderDispatched
	}

	h.reassign.CancelRetry(a.OrderID)
	kit.OK(c, gin.H{"assignment_id": a.ID, "status": a.Status})
}

// RejectAssignment handles POST /dispatch/driver/reject.
func (h *DispatchHandler) RejectAssignment(c *gin.Context) {
	var req model.RejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		kit.BadRequest(c, "assignment_id, driver_id, order_id required")
		return
	}

	// Validate driver
	_, ok := h.stateMachine.GetDriver(req.DriverID)
	if !ok {
		kit.BadRequest(c, "driver not found")
		return
	}

	// Update assignment status
	h.mu.RLock()
	a, ok := h.assignments[req.AssignmentID]
	h.mu.RUnlock()
	if ok {
		now := time.Now()
		a.Status = "REJECTED"
		a.RejectedAt = &now
	}

	// Trigger reassignment
	if err := h.reassign.HandleReject(req.DriverID, req.OrderID); err != nil {
		kit.InternalError(c, "reassignment failed: "+err.Error())
		return
	}

	kit.OK(c, gin.H{"reassigned": true})
}

// GetOrder handles GET /dispatch/orders/:id.
func (h *DispatchHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")

	h.mu.RLock()
	order, ok := h.orders[orderID]
	h.mu.RUnlock()

	if !ok {
		kit.NotFound(c, "order not found")
		return
	}
	kit.OK(c, order)
}

// Register adds all dispatch endpoints to the provided router group.
func (h *DispatchHandler) Register(r *gin.RouterGroup) {
	r.POST("/dispatch/order", h.CreateOrder)
	r.POST("/dispatch/driver/location", h.UpdateDriverLocation)
	r.POST("/dispatch/driver/accept", h.AcceptAssignment)
	r.POST("/dispatch/driver/reject", h.RejectAssignment)
	r.GET("/dispatch/orders/:id", h.GetOrder)
}
