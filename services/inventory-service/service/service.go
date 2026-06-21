package service

import (
	"context"
	"fmt"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/model"
	"github.com/faisalaffan/faisalaffan-design-system/services/inventory-service/repository"
	"github.com/google/uuid"
)

type InventoryService struct {
	repo *repository.InventoryRepo
}

func NewInventoryService(repo *repository.InventoryRepo) *InventoryService {
	return &InventoryService{repo: repo}
}

func (s *InventoryService) GetStock(ctx context.Context, hubID, sku string) (*model.StockInfo, error) {
	available, err := s.repo.GetStock(ctx, hubID, sku)
	if err != nil {
		return nil, err
	}
	return &model.StockInfo{
		HubID:     hubID,
		SKU:       sku,
		Available: available,
	}, nil
}

func (s *InventoryService) Reserve(ctx context.Context, req *model.ReserveRequest) (*model.ReserveResponse, error) {
	reservationID := uuid.New().String()
	now := time.Now()
	ttl := 15 * time.Minute

	var items []model.ReservedItem
	for _, item := range req.Items {
		remaining, err := s.repo.Reserve(ctx, req.HubID, item.SKU, item.Quantity, reservationID, req.UserID, req.CartID, ttl)
		if err != nil {
			// Rollback previous reservations
			for _, prev := range items {
				s.repo.Release(ctx, reservationID, req.HubID, prev.SKU)
			}
			return nil, fmt.Errorf("reserve %s: %w", item.SKU, err)
		}
		items = append(items, model.ReservedItem{
			SKU: item.SKU, Quantity: item.Quantity, Remaining: remaining,
		})
	}

	return &model.ReserveResponse{
		ReservationID: reservationID,
		Status:        "RESERVED",
		Items:         items,
		ExpiresAt:     now.Add(ttl).UnixMilli(),
	}, nil
}

func (s *InventoryService) Release(ctx context.Context, reservationID, hubID, sku string) error {
	return s.repo.Release(ctx, reservationID, hubID, sku)
}

func (s *InventoryService) Confirm(ctx context.Context, reservationID, hubID, sku string) error {
	return s.repo.ConfirmReservation(ctx, reservationID, hubID, sku)
}

func (s *InventoryService) SetStock(ctx context.Context, hubID, sku string, qty int) error {
	return s.repo.SetStock(ctx, hubID, sku, qty)
}
