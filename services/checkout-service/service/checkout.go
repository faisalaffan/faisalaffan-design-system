package service

import (
	"fmt"
	"log"

	"github.com/faisalaffan/faisalaffan-design-system/services/checkout-service/model"
	"github.com/google/uuid"
)

// CheckoutService orchestrates the full checkout flow.
type CheckoutService struct {
	saga       *SagaOrchestrator
	createStep *CreateOrderStep
	outbox     chan<- model.OutboxEvent
}

// NewCheckoutService creates a service wired with the saga pipeline.
func NewCheckoutService(outbox chan<- model.OutboxEvent) *CheckoutService {
	createStep := NewCreateOrderStep()
	confirmStep := NewConfirmOrderStep(createStep)
	outboxStep := NewPublishEventsStep(outbox)

	steps := []StepHandler{
		&ReserveInventoryStep{},
		createStep,
		&ChargePaymentStep{},
		confirmStep,
		outboxStep,
	}

	return &CheckoutService{
		saga:       NewSagaOrchestrator(steps),
		createStep: createStep,
		outbox:     outbox,
	}
}

// NewOrder runs the full checkout saga for the given request.
// Returns the saga result with the created order on success.
func (s *CheckoutService) NewOrder(ctx *model.SagaContext) (*model.SagaResult, error) {
	ctx.OrderID = uuid.New().String()

	// Calculate total from items.
	var total int64
	for _, item := range ctx.Items {
		total += item.UnitPrice * int64(item.Quantity)
	}
	ctx.Total = total

	if err := s.saga.Execute(ctx); err != nil {
		log.Printf("checkout: order %s failed: %v", ctx.OrderID, err)
		return &model.SagaResult{
			Success:  false,
			OrderID:  ctx.OrderID,
			FailedAt: ctx.FailedAt,
			Steps:    ctx.Steps,
		}, nil // return result, not the error — caller inspects the result
	}

	order := s.createStep.GetOrder(ctx.OrderID)
	log.Printf("checkout: order %s completed successfully", ctx.OrderID)
	return &model.SagaResult{
		Success: true,
		OrderID: ctx.OrderID,
		Order:   order,
		Steps:   ctx.Steps,
	}, nil
}

// ConfirmPayment handles a successful payment webhook.
// It updates the order status in the in-memory store.
func (s *CheckoutService) ConfirmPayment(orderID string) error {
	order := s.createStep.GetOrder(orderID)
	if order == nil {
		return fmt.Errorf("order %s not found", orderID)
	}
	if order.Status != model.OrderStatusConfirmed {
		return fmt.Errorf("order %s status is %s, expected confirmed", orderID, order.Status)
	}

	// Payment already charged during saga; the webhook is just confirmation.
	log.Printf("checkout: payment confirmed for order %s", orderID)

	evt := model.OutboxEvent{
		ID:        uuid.New().String(),
		OrderID:   orderID,
		EventType: "payment.confirmed",
		Payload: map[string]interface{}{
			"order_id": orderID,
		},
	}
	s.outbox <- evt
	return nil
}

// GetOrder returns a stored order by ID.
func (s *CheckoutService) GetOrder(id string) *model.Order {
	return s.createStep.GetOrder(id)
}
