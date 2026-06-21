package model

type Reservation struct {
	ReservationID string `json:"reservation_id"`
	HubID         string `json:"hub_id"`
	SKU           string `json:"sku"`
	Quantity      int    `json:"quantity"`
	UserID        string `json:"user_id"`
	CartID        string `json:"cart_id"`
	Status        string `json:"status"` // RESERVED, CONFIRMED, RELEASED, EXPIRED
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
}

type ReserveRequest struct {
	HubID  string        `json:"hub_id" binding:"required"`
	CartID string        `json:"cart_id" binding:"required"`
	UserID string        `json:"user_id" binding:"required"`
	Items  []ReserveItem `json:"items" binding:"required"`
}

type ReserveItem struct {
	SKU      string `json:"sku" binding:"required"`
	Quantity int    `json:"quantity" binding:"required,min=1,max=50"`
}

type ReserveResponse struct {
	ReservationID string         `json:"reservation_id"`
	Status        string         `json:"status"`
	Items         []ReservedItem `json:"items"`
	ExpiresAt     int64          `json:"expires_at"`
}

type ReservedItem struct {
	SKU       string `json:"sku"`
	Quantity  int    `json:"quantity"`
	Remaining int    `json:"remaining"`
}

type StockInfo struct {
	HubID     string `json:"hub_id"`
	SKU       string `json:"sku"`
	Available int    `json:"available"`
	Reserved  int    `json:"reserved"`
}

type ReleaseRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}

type ConfirmRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}

type CycleCountRequest struct {
	HubID            string `json:"hub_id" binding:"required"`
	SKU              string `json:"sku" binding:"required"`
	PhysicalQuantity int    `json:"physical_quantity" binding:"required"`
	CountedBy        string `json:"counted_by" binding:"required"`
}
