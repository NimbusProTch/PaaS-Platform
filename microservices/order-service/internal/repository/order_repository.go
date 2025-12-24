package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/infraforge/order-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type OrderRepository interface {
	Create(ctx context.Context, order *models.Order) error
	GetByID(ctx context.Context, id string) (*models.Order, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]models.Order, error)
	Update(ctx context.Context, order *models.Order) error
	UpdateStatus(ctx context.Context, id string, status models.OrderStatus) error
	Delete(ctx context.Context, id string) error
	GetByStatus(ctx context.Context, status models.OrderStatus) ([]models.Order, error)
	GetPendingOrders(ctx context.Context, duration time.Duration) ([]models.Order, error)
}

type orderRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewOrderRepository(db *gorm.DB, redis *redis.Client) OrderRepository {
	return &orderRepository{
		db:    db,
		redis: redis,
	}
}

func (r *orderRepository) Create(ctx context.Context, order *models.Order) error {
	// Start transaction
	tx := r.db.WithContext(ctx).Begin()

	// Create order
	if err := tx.Create(order).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Create order items
	for _, item := range order.Items {
		item.OrderID = order.ID
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create order item: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	r.invalidateCache(ctx, order.ID, order.UserID)

	return nil
}

func (r *orderRepository) GetByID(ctx context.Context, id string) (*models.Order, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("order:%s", id)
	cached, err := r.redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var order models.Order
		if err := json.Unmarshal([]byte(cached), &order); err == nil {
			return &order, nil
		}
	}

	// Query database
	var order models.Order
	err = r.db.WithContext(ctx).
		Preload("Items").
		First(&order, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Cache result
	r.cacheOrder(ctx, &order)

	return &order, nil
}

func (r *orderRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]models.Order, error) {
	var orders []models.Order

	query := r.db.WithContext(ctx).
		Preload("Items").
		Where("user_id = ?", userID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("failed to get user orders: %w", err)
	}

	return orders, nil
}

func (r *orderRepository) Update(ctx context.Context, order *models.Order) error {
	tx := r.db.WithContext(ctx).Begin()

	// Update order
	if err := tx.Model(order).Updates(order).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update order: %w", err)
	}

	// Delete existing items
	if err := tx.Where("order_id = ?", order.ID).Delete(&models.OrderItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete order items: %w", err)
	}

	// Create new items
	for _, item := range order.Items {
		item.OrderID = order.ID
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create order item: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	r.invalidateCache(ctx, order.ID, order.UserID)

	return nil
}

func (r *orderRepository) UpdateStatus(ctx context.Context, id string, status models.OrderStatus) error {
	result := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update order status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("order not found")
	}

	// Invalidate cache
	r.redis.Del(ctx, fmt.Sprintf("order:%s", id))

	return nil
}

func (r *orderRepository) Delete(ctx context.Context, id string) error {
	tx := r.db.WithContext(ctx).Begin()

	// Delete order items first
	if err := tx.Where("order_id = ?", id).Delete(&models.OrderItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete order items: %w", err)
	}

	// Delete order
	result := tx.Delete(&models.Order{}, "id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete order: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		return fmt.Errorf("order not found")
	}

	tx.Commit()

	// Invalidate cache
	r.redis.Del(ctx, fmt.Sprintf("order:%s", id))

	return nil
}

func (r *orderRepository) GetByStatus(ctx context.Context, status models.OrderStatus) ([]models.Order, error) {
	var orders []models.Order

	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("status = ?", status).
		Find(&orders).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get orders by status: %w", err)
	}

	return orders, nil
}

func (r *orderRepository) GetPendingOrders(ctx context.Context, duration time.Duration) ([]models.Order, error) {
	var orders []models.Order

	cutoff := time.Now().Add(-duration)

	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("status = ? AND created_at < ?", models.OrderStatusPending, cutoff).
		Find(&orders).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get pending orders: %w", err)
	}

	return orders, nil
}

// Helper methods
func (r *orderRepository) cacheOrder(ctx context.Context, order *models.Order) {
	data, _ := json.Marshal(order)
	cacheKey := fmt.Sprintf("order:%s", order.ID)
	r.redis.Set(ctx, cacheKey, data, 10*time.Minute)
}

func (r *orderRepository) invalidateCache(ctx context.Context, orderID, userID string) {
	keys := []string{
		fmt.Sprintf("order:%s", orderID),
		fmt.Sprintf("user_orders:%s", userID),
	}
	for _, key := range keys {
		r.redis.Del(ctx, key)
	}
}