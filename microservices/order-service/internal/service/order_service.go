package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/infraforge/order-service/internal/models"
	"github.com/infraforge/order-service/internal/repository"
	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("order-service")

type OrderService interface {
	CreateOrder(ctx context.Context, order *models.Order) error
	GetOrder(ctx context.Context, id string) (*models.Order, error)
	GetUserOrders(ctx context.Context, userID string, page, limit int) ([]models.Order, int, error)
	UpdateOrder(ctx context.Context, order *models.Order) error
	UpdateOrderStatus(ctx context.Context, id string, status models.OrderStatus) error
	CancelOrder(ctx context.Context, id string) error
	GetOrdersByStatus(ctx context.Context, status models.OrderStatus) ([]models.Order, error)
	ProcessPendingOrders(ctx context.Context) error
	CalculateOrderTotal(order *models.Order) float64
}

type orderService struct {
	repo       repository.OrderRepository
	amqpChan   *amqp.Channel
	tracer     trace.Tracer
}

func NewOrderService(repo repository.OrderRepository, amqpChan *amqp.Channel) OrderService {
	return &orderService{
		repo:     repo,
		amqpChan: amqpChan,
		tracer:   tracer,
	}
}

func (s *orderService) CreateOrder(ctx context.Context, order *models.Order) error {
	ctx, span := s.tracer.Start(ctx, "CreateOrder")
	defer span.End()

	// Validate order
	if err := s.validateOrder(order); err != nil {
		return fmt.Errorf("invalid order: %w", err)
	}

	// Calculate total
	order.CalculateTotal()

	// Set initial status
	if order.Status == "" {
		order.Status = models.OrderStatusPending
	}

	// Create order in database
	if err := s.repo.Create(ctx, order); err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Publish order created event
	if err := s.publishOrderEvent(ctx, "order.created", order); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to publish order created event: %v\n", err)
	}

	return nil
}

func (s *orderService) GetOrder(ctx context.Context, id string) (*models.Order, error) {
	ctx, span := s.tracer.Start(ctx, "GetOrder")
	defer span.End()

	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return order, nil
}

func (s *orderService) GetUserOrders(ctx context.Context, userID string, page, limit int) ([]models.Order, int, error) {
	ctx, span := s.tracer.Start(ctx, "GetUserOrders")
	defer span.End()

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	orders, err := s.repo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user orders: %w", err)
	}

	// Get total count for pagination
	totalOrders, err := s.repo.GetByUserID(ctx, userID, 0, 0)
	if err != nil {
		return orders, 0, nil
	}

	return orders, len(totalOrders), nil
}

func (s *orderService) UpdateOrder(ctx context.Context, order *models.Order) error {
	ctx, span := s.tracer.Start(ctx, "UpdateOrder")
	defer span.End()

	// Validate order
	if err := s.validateOrder(order); err != nil {
		return fmt.Errorf("invalid order: %w", err)
	}

	// Get existing order
	existing, err := s.repo.GetByID(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	// Check if order can be updated
	if existing.Status != models.OrderStatusPending && existing.Status != models.OrderStatusConfirmed {
		return fmt.Errorf("order cannot be updated in status: %s", existing.Status)
	}

	// Recalculate total
	order.CalculateTotal()

	// Update order
	if err := s.repo.Update(ctx, order); err != nil {
		return fmt.Errorf("failed to update order: %w", err)
	}

	// Publish order updated event
	if err := s.publishOrderEvent(ctx, "order.updated", order); err != nil {
		fmt.Printf("Failed to publish order updated event: %v\n", err)
	}

	return nil
}

func (s *orderService) UpdateOrderStatus(ctx context.Context, id string, status models.OrderStatus) error {
	ctx, span := s.tracer.Start(ctx, "UpdateOrderStatus")
	defer span.End()

	// Get existing order
	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	// Validate status transition
	if !s.isValidStatusTransition(order.Status, status) {
		return fmt.Errorf("invalid status transition from %s to %s", order.Status, status)
	}

	// Update status
	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	// Update timestamps based on status
	now := time.Now()
	switch status {
	case models.OrderStatusShipped:
		order.ShippedAt = &now
	case models.OrderStatusDelivered:
		order.DeliveredAt = &now
	}

	// Publish status changed event
	order.Status = status
	if err := s.publishOrderEvent(ctx, fmt.Sprintf("order.status.%s", status), order); err != nil {
		fmt.Printf("Failed to publish order status event: %v\n", err)
	}

	return nil
}

func (s *orderService) CancelOrder(ctx context.Context, id string) error {
	ctx, span := s.tracer.Start(ctx, "CancelOrder")
	defer span.End()

	// Get order
	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	// Check if order can be cancelled
	if !order.CanCancel() {
		return fmt.Errorf("order cannot be cancelled in status: %s", order.Status)
	}

	// Update status to cancelled
	if err := s.repo.UpdateStatus(ctx, id, models.OrderStatusCancelled); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	// Publish order cancelled event
	order.Status = models.OrderStatusCancelled
	if err := s.publishOrderEvent(ctx, "order.cancelled", order); err != nil {
		fmt.Printf("Failed to publish order cancelled event: %v\n", err)
	}

	return nil
}

func (s *orderService) GetOrdersByStatus(ctx context.Context, status models.OrderStatus) ([]models.Order, error) {
	ctx, span := s.tracer.Start(ctx, "GetOrdersByStatus")
	defer span.End()

	orders, err := s.repo.GetByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders by status: %w", err)
	}

	return orders, nil
}

func (s *orderService) ProcessPendingOrders(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "ProcessPendingOrders")
	defer span.End()

	// Get orders pending for more than 30 minutes
	pendingOrders, err := s.repo.GetPendingOrders(ctx, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to get pending orders: %w", err)
	}

	for _, order := range pendingOrders {
		// Auto-cancel orders pending for too long
		if err := s.CancelOrder(ctx, order.ID); err != nil {
			fmt.Printf("Failed to auto-cancel order %s: %v\n", order.ID, err)
			continue
		}
	}

	return nil
}

func (s *orderService) CalculateOrderTotal(order *models.Order) float64 {
	order.CalculateTotal()
	return order.TotalPrice
}

// Helper methods
func (s *orderService) validateOrder(order *models.Order) error {
	if order.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if len(order.Items) == 0 {
		return fmt.Errorf("order must have at least one item")
	}

	for _, item := range order.Items {
		if item.ProductID == "" {
			return fmt.Errorf("product ID is required for all items")
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("quantity must be greater than 0")
		}
		if item.Price < 0 {
			return fmt.Errorf("price cannot be negative")
		}
	}

	// Validate addresses
	if order.ShippingAddress.Street == "" || order.ShippingAddress.City == "" {
		return fmt.Errorf("shipping address is incomplete")
	}

	return nil
}

func (s *orderService) isValidStatusTransition(from, to models.OrderStatus) bool {
	validTransitions := map[models.OrderStatus][]models.OrderStatus{
		models.OrderStatusPending: {
			models.OrderStatusConfirmed,
			models.OrderStatusCancelled,
		},
		models.OrderStatusConfirmed: {
			models.OrderStatusProcessing,
			models.OrderStatusCancelled,
		},
		models.OrderStatusProcessing: {
			models.OrderStatusShipped,
			models.OrderStatusCancelled,
		},
		models.OrderStatusShipped: {
			models.OrderStatusDelivered,
		},
		models.OrderStatusDelivered: {
			models.OrderStatusRefunded,
		},
		models.OrderStatusCancelled: {},
		models.OrderStatusRefunded:  {},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

func (s *orderService) publishOrderEvent(ctx context.Context, eventType string, order *models.Order) error {
	if s.amqpChan == nil {
		return nil // Skip if no RabbitMQ connection
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"order_id":   order.ID,
		"user_id":    order.UserID,
		"status":     order.Status,
		"total":      order.TotalPrice,
		"timestamp":  time.Now().Unix(),
		"order":      order,
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = s.amqpChan.Publish(
		"orders",   // exchange
		eventType,  // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}