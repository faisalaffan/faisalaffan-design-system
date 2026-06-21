package model

import "time"

// Status constants for Order.
const (
	OrderStatusPending   = "pending"
	OrderStatusConfirmed = "confirmed"
	OrderStatusCancelled = "cancelled"
	OrderStatusFailed    = "failed"
)

// SagaStepName identifies a step in the saga.
type SagaStepName string

const (
	StepReserveInventory SagaStepName = "reserve_inventory"
	StepCreateOrder      SagaStepName = "create_order"
	StepChargePayment    SagaStepName = "charge_payment"
	StepConfirmOrder     SagaStepName = "confirm_order"
	StepPublishEvents    SagaStepName = "publish_events"
)

// Order represents a checkout order.
type Order struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	Status     string       `json:"status"`
	Items      []OrderItem  `json:"items"`
	Total      int64        `json:"total"`
	Currency   string       `json:"currency"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// OrderItem is a single line item within an order.
type OrderItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
}

// CheckoutRequest is the incoming POST /checkout payload.
type CheckoutRequest struct {
	UserID  string      `json:"user_id"  binding:"required"`
	Items   []OrderItem `json:"items"    binding:"required,min=1"`
	Payment PaymentInfo `json:"payment"  binding:"required"`
}

// PaymentInfo contains payment details.
type PaymentInfo struct {
	Method       string `json:"method"`
	CardToken    string `json:"card_token,omitempty"`
	Currency     string `json:"currency"     binding:"required"`
}

// CheckoutResponse is returned after a successful saga run.
type CheckoutResponse struct {
	OrderID     string `json:"order_id"`
	Status      string `json:"status"`
	Transaction string `json:"transaction,omitempty"`
}

// WebhookPayload is the generic payment webhook payload.
type WebhookPayload struct {
	TransactionID string `json:"transaction_id"`
	OrderID       string `json:"order_id"`
	Status        string `json:"status"` // "succeeded" | "failed"
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
}

// SagaStep holds the result of a single saga step execution.
type SagaStep struct {
	Name     SagaStepName `json:"name"`
	Success  bool         `json:"success"`
	Result   interface{}  `json:"result,omitempty"`
	Error    string       `json:"error,omitempty"`
}

// SagaContext carries data across saga steps.
type SagaContext struct {
	OrderID      string        `json:"order_id"`
	UserID       string        `json:"user_id"`
	Items        []OrderItem   `json:"items"`
	Total        int64         `json:"total"`
	Currency     string        `json:"currency"`
	TransactionID string       `json:"transaction_id,omitempty"`
	Steps        []SagaStep    `json:"steps"`
	FailedAt     *SagaStepName `json:"failed_at,omitempty"`
}

// SagaResult is the final outcome of a saga run.
type SagaResult struct {
	Success   bool       `json:"success"`
	OrderID   string     `json:"order_id,omitempty"`
	Order     *Order     `json:"order,omitempty"`
	FailedAt  *SagaStepName `json:"failed_at,omitempty"`
	Steps     []SagaStep `json:"steps"`
}

// IdempotencyKey is stored in Redis to guarantee at-most-once execution.
type IdempotencyKey struct {
	Key       string `json:"key"`
	Result    interface{} `json:"result"`
	CreatedAt int64  `json:"created_at"`
}

// OutboxEvent is published after a successful saga.
type OutboxEvent struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"order_id"`
	EventType string    `json:"event_type"`
	Payload   interface{} `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
