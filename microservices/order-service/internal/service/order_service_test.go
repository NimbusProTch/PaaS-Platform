package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/infraforge/order-service/internal/models"
	"github.com/infraforge/order-service/internal/service"
)

// Mock repository
type mockOrderRepository struct {
	orders map[string]*models.Order
	err    error
}

func newMockRepository() *mockOrderRepository {
	return &mockOrderRepository{
		orders: make(map[string]*models.Order),
	}
}

func (m *mockOrderRepository) Create(ctx context.Context, order *models.Order) error {
	if m.err != nil {
		return m.err
	}
	if order.ID == "" {
		order.ID = "test-order-id"
	}
	m.orders[order.ID] = order
	return nil
}

func (m *mockOrderRepository) GetByID(ctx context.Context, id string) (*models.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	order, exists := m.orders[id]
	if !exists {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (m *mockOrderRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]models.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	var orders []models.Order
	for _, order := range m.orders {
		if order.UserID == userID {
			orders = append(orders, *order)
		}
	}
	return orders, nil
}

func (m *mockOrderRepository) Update(ctx context.Context, order *models.Order) error {
	if m.err != nil {
		return m.err
	}
	m.orders[order.ID] = order
	return nil
}

func (m *mockOrderRepository) UpdateStatus(ctx context.Context, id string, status models.OrderStatus) error {
	if m.err != nil {
		return m.err
	}
	order, exists := m.orders[id]
	if !exists {
		return errors.New("order not found")
	}
	order.Status = status
	return nil
}

func (m *mockOrderRepository) Delete(ctx context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.orders, id)
	return nil
}

func (m *mockOrderRepository) GetByStatus(ctx context.Context, status models.OrderStatus) ([]models.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	var orders []models.Order
	for _, order := range m.orders {
		if order.Status == status {
			orders = append(orders, *order)
		}
	}
	return orders, nil
}

func (m *mockOrderRepository) GetPendingOrders(ctx context.Context, duration time.Duration) ([]models.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	var orders []models.Order
	cutoff := time.Now().Add(-duration)
	for _, order := range m.orders {
		if order.Status == models.OrderStatusPending && order.CreatedAt.Before(cutoff) {
			orders = append(orders, *order)
		}
	}
	return orders, nil
}

// Tests
func TestCreateOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	order := &models.Order{
		UserID: "user-123",
		Items: []models.OrderItem{
			{
				ProductID: "product-1",
				Quantity:  2,
				Price:     10.00,
			},
		},
		ShippingAddress: models.Address{
			Street:  "123 Main St",
			City:    "New York",
			Country: "USA",
		},
		BillingAddress: models.Address{
			Street:  "123 Main St",
			City:    "New York",
			Country: "USA",
		},
	}

	err := svc.CreateOrder(ctx, order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	if order.ID == "" {
		t.Error("Order ID should be generated")
	}

	if order.Status != models.OrderStatusPending {
		t.Errorf("Expected status to be pending, got %s", order.Status)
	}

	if order.TotalPrice != 20.00 {
		t.Errorf("Expected total price to be 20.00, got %f", order.TotalPrice)
	}
}

func TestCreateOrderValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	tests := []struct {
		name  string
		order *models.Order
		want  bool
	}{
		{
			name: "missing user ID",
			order: &models.Order{
				Items: []models.OrderItem{{ProductID: "p1", Quantity: 1, Price: 10}},
				ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
			},
			want: true,
		},
		{
			name: "no items",
			order: &models.Order{
				UserID:          "user-1",
				Items:           []models.OrderItem{},
				ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
			},
			want: true,
		},
		{
			name: "invalid quantity",
			order: &models.Order{
				UserID: "user-1",
				Items: []models.OrderItem{
					{ProductID: "p1", Quantity: 0, Price: 10},
				},
				ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
			},
			want: true,
		},
		{
			name: "negative price",
			order: &models.Order{
				UserID: "user-1",
				Items: []models.OrderItem{
					{ProductID: "p1", Quantity: 1, Price: -10},
				},
				ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CreateOrder(ctx, tt.order)
			if (err != nil) != tt.want {
				t.Errorf("CreateOrder() error = %v, want error %v", err, tt.want)
			}
		})
	}
}

func TestGetOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	// Create an order first
	order := &models.Order{
		ID:     "order-123",
		UserID: "user-123",
		Items: []models.OrderItem{
			{ProductID: "p1", Quantity: 1, Price: 10},
		},
		ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
	}
	repo.orders[order.ID] = order

	// Get the order
	retrieved, err := svc.GetOrder(ctx, "order-123")
	if err != nil {
		t.Fatalf("Failed to get order: %v", err)
	}

	if retrieved.ID != order.ID {
		t.Errorf("Expected order ID %s, got %s", order.ID, retrieved.ID)
	}

	// Test non-existent order
	_, err = svc.GetOrder(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error for non-existent order")
	}
}

func TestUpdateOrderStatus(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	// Create an order
	order := &models.Order{
		ID:     "order-123",
		UserID: "user-123",
		Status: models.OrderStatusPending,
		Items: []models.OrderItem{
			{ProductID: "p1", Quantity: 1, Price: 10},
		},
		ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
	}
	repo.orders[order.ID] = order

	// Valid status transition
	err := svc.UpdateOrderStatus(ctx, order.ID, models.OrderStatusConfirmed)
	if err != nil {
		t.Fatalf("Failed to update order status: %v", err)
	}

	if repo.orders[order.ID].Status != models.OrderStatusConfirmed {
		t.Error("Order status was not updated")
	}

	// Invalid status transition
	err = svc.UpdateOrderStatus(ctx, order.ID, models.OrderStatusDelivered)
	if err == nil {
		t.Error("Expected error for invalid status transition")
	}
}

func TestCancelOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	// Create a pending order
	order := &models.Order{
		ID:     "order-123",
		UserID: "user-123",
		Status: models.OrderStatusPending,
		Items: []models.OrderItem{
			{ProductID: "p1", Quantity: 1, Price: 10},
		},
		ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
	}
	repo.orders[order.ID] = order

	// Cancel the order
	err := svc.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("Failed to cancel order: %v", err)
	}

	if repo.orders[order.ID].Status != models.OrderStatusCancelled {
		t.Error("Order was not cancelled")
	}

	// Try to cancel an already delivered order
	deliveredOrder := &models.Order{
		ID:     "order-456",
		UserID: "user-123",
		Status: models.OrderStatusDelivered,
		Items: []models.OrderItem{
			{ProductID: "p1", Quantity: 1, Price: 10},
		},
		ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
	}
	repo.orders[deliveredOrder.ID] = deliveredOrder

	err = svc.CancelOrder(ctx, deliveredOrder.ID)
	if err == nil {
		t.Error("Expected error when cancelling delivered order")
	}
}

func TestCalculateOrderTotal(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	order := &models.Order{
		Items: []models.OrderItem{
			{ProductID: "p1", Quantity: 2, Price: 10.50},
			{ProductID: "p2", Quantity: 1, Price: 5.25},
			{ProductID: "p3", Quantity: 3, Price: 2.00},
		},
	}

	total := svc.CalculateOrderTotal(order)
	expected := 32.25 // (2*10.50) + (1*5.25) + (3*2.00)

	if total != expected {
		t.Errorf("Expected total %f, got %f", expected, total)
	}
}

func TestGetUserOrders(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	svc := service.NewOrderService(repo, nil)

	// Create multiple orders for a user
	userID := "user-123"
	for i := 0; i < 5; i++ {
		order := &models.Order{
			ID:     fmt.Sprintf("order-%d", i),
			UserID: userID,
			Status: models.OrderStatusPending,
			Items: []models.OrderItem{
				{ProductID: "p1", Quantity: 1, Price: 10},
			},
			ShippingAddress: models.Address{Street: "123 Main", City: "NY"},
		}
		repo.orders[order.ID] = order
	}

	// Get user orders
	orders, total, err := svc.GetUserOrders(ctx, userID, 1, 10)
	if err != nil {
		t.Fatalf("Failed to get user orders: %v", err)
	}

	if len(orders) != 5 {
		t.Errorf("Expected 5 orders, got %d", len(orders))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}