package service

import (
	"fmt"
	"log"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/model"
	"github.com/google/uuid"
)

// StepHandler defines the interface for a single saga step.
// Each step implements Execute and optionally Compensate.
type StepHandler interface {
	Name() model.SagaStepName
	Execute(ctx *model.SagaContext) error
	Compensate(ctx *model.SagaContext) error
}

// SagaOrchestrator runs steps sequentially and compensates on failure.
type SagaOrchestrator struct {
	steps []StepHandler
}

// NewSagaOrchestrator creates an orchestrator with the given ordered steps.
func NewSagaOrchestrator(steps []StepHandler) *SagaOrchestrator {
	return &SagaOrchestrator{steps: steps}
}

// Execute runs every step in order. If a step fails, it compensates all
// previously completed steps in reverse order and returns the wrapped error.
func (o *SagaOrchestrator) Execute(ctx *model.SagaContext) error {
	completed := make([]StepHandler, 0, len(o.steps))

	for _, step := range o.steps {
		stepName := step.Name()
		if err := step.Execute(ctx); err != nil {
			log.Printf("saga: step %s failed: %v", stepName, err)

			ctx.Steps = append(ctx.Steps, model.SagaStep{
				Name:    stepName,
				Success: false,
				Error:   err.Error(),
			})
			failed := stepName
			ctx.FailedAt = &failed

			o.compensate(completed, ctx)
			return fmt.Errorf("saga failed at step %s: %w", stepName, err)
		}

		log.Printf("saga: step %s succeeded", stepName)
		ctx.Steps = append(ctx.Steps, model.SagaStep{
			Name:    stepName,
			Success: true,
		})
		completed = append(completed, step)
	}

	return nil
}

func (o *SagaOrchestrator) compensate(completed []StepHandler, ctx *model.SagaContext) {
	for i := len(completed) - 1; i >= 0; i-- {
		comp := completed[i]
		compName := comp.Name()
		if cerr := comp.Compensate(ctx); cerr != nil {
			log.Printf("saga: compensation for step %s failed: %v", compName, cerr)
		} else {
			log.Printf("saga: compensated step %s", compName)
		}
	}
}

// ─── Step Implementations ────────────────────────────────────────────────────

// ReserveInventoryStep reserves stock for each item.
type ReserveInventoryStep struct{}

func (s *ReserveInventoryStep) Name() model.SagaStepName { return model.StepReserveInventory }

func (s *ReserveInventoryStep) Execute(ctx *model.SagaContext) error {
	for _, item := range ctx.Items {
		log.Printf("inventory: reserving %d x %s", item.Quantity, item.ProductID)
		// TODO: call inventory-service gRPC/REST
	}
	return nil
}

func (s *ReserveInventoryStep) Compensate(ctx *model.SagaContext) error {
	for _, item := range ctx.Items {
		log.Printf("inventory: releasing %d x %s", item.Quantity, item.ProductID)
		// TODO: call inventory-service gRPC/REST
	}
	return nil
}

// CreateOrderStep persists the order record.
type CreateOrderStep struct {
	orders map[string]*model.Order // in-memory store (replace with DB)
}

func NewCreateOrderStep() *CreateOrderStep {
	return &CreateOrderStep{orders: make(map[string]*model.Order)}
}

func (s *CreateOrderStep) Name() model.SagaStepName { return model.StepCreateOrder }

func (s *CreateOrderStep) Execute(ctx *model.SagaContext) error {
	now := time.Now()
	order := &model.Order{
		ID:        ctx.OrderID,
		UserID:    ctx.UserID,
		Status:    model.OrderStatusPending,
		Items:     ctx.Items,
		Total:     ctx.Total,
		Currency:  ctx.Currency,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.orders[ctx.OrderID] = order
	log.Printf("order: created %s (status=%s)", order.ID, order.Status)
	return nil
}

func (s *CreateOrderStep) Compensate(ctx *model.SagaContext) error {
	if o, ok := s.orders[ctx.OrderID]; ok {
		o.Status = model.OrderStatusCancelled
		log.Printf("order: cancelled %s", o.ID)
	}
	return nil
}

// GetOrder returns the persisted order, or nil.
func (s *CreateOrderStep) GetOrder(id string) *model.Order {
	return s.orders[id]
}

// ChargePaymentStep charges the payment method.
type ChargePaymentStep struct{}

func (s *ChargePaymentStep) Name() model.SagaStepName { return model.StepChargePayment }

func (s *ChargePaymentStep) Execute(ctx *model.SagaContext) error {
	txnID := fmt.Sprintf("txn_%s", ctx.OrderID)
	ctx.TransactionID = txnID
	log.Printf("payment: charged %d %s (txn=%s)", ctx.Total, ctx.Currency, txnID)
	// TODO: call payment-gateway API
	return nil
}

func (s *ChargePaymentStep) Compensate(ctx *model.SagaContext) error {
	if ctx.TransactionID != "" {
		log.Printf("payment: refunding txn=%s", ctx.TransactionID)
		// TODO: call payment-gateway refund API
	}
	return nil
}

// ConfirmOrderStep marks the order as confirmed.
type ConfirmOrderStep struct {
	createStep *CreateOrderStep
}

func NewConfirmOrderStep(cs *CreateOrderStep) *ConfirmOrderStep {
	return &ConfirmOrderStep{createStep: cs}
}

func (s *ConfirmOrderStep) Name() model.SagaStepName { return model.StepConfirmOrder }

func (s *ConfirmOrderStep) Execute(ctx *model.SagaContext) error {
	o := s.createStep.GetOrder(ctx.OrderID)
	if o == nil {
		return fmt.Errorf("order %s not found", ctx.OrderID)
	}
	o.Status = model.OrderStatusConfirmed
	o.UpdatedAt = time.Now()
	log.Printf("order: confirmed %s", o.ID)
	return nil
}

func (s *ConfirmOrderStep) Compensate(ctx *model.SagaContext) error {
	o := s.createStep.GetOrder(ctx.OrderID)
	if o != nil {
		o.Status = model.OrderStatusFailed
		o.UpdatedAt = time.Now()
		log.Printf("order: reverted confirmation for %s", o.ID)
	}
	return nil
}

// PublishEventsStep writes outbox events to a channel after saga success.
type PublishEventsStep struct {
	outbox chan<- model.OutboxEvent
}

func NewPublishEventsStep(outbox chan<- model.OutboxEvent) *PublishEventsStep {
	return &PublishEventsStep{outbox: outbox}
}

func (s *PublishEventsStep) Name() model.SagaStepName { return model.StepPublishEvents }

func (s *PublishEventsStep) Execute(ctx *model.SagaContext) error {
	events := []model.OutboxEvent{
		{
			ID:        uuid.New().String(),
			OrderID:   ctx.OrderID,
			EventType: "order.created",
			Payload:   ctx,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New().String(),
			OrderID:   ctx.OrderID,
			EventType: "payment.charged",
			Payload: map[string]interface{}{
				"order_id":       ctx.OrderID,
				"transaction_id": ctx.TransactionID,
				"amount":         ctx.Total,
				"currency":       ctx.Currency,
			},
			CreatedAt: time.Now(),
		},
	}
	for _, evt := range events {
		s.outbox <- evt
		log.Printf("outbox: enqueued %s event for order %s", evt.EventType, evt.OrderID)
	}
	return nil
}

func (s *PublishEventsStep) Compensate(ctx *model.SagaContext) error {
	// Publishing events has no side-effect that needs local compensation;
	// downstream consumers should handle idempotent consumption.
	log.Printf("outbox: no compensation for events (order=%s)", ctx.OrderID)
	return nil
}
