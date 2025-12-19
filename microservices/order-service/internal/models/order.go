package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusRefunded   OrderStatus = "refunded"
)

type Order struct {
	ID         string      `json:"id" gorm:"primaryKey"`
	UserID     string      `json:"user_id" gorm:"not null;index"`
	Status     OrderStatus `json:"status" gorm:"default:pending"`
	TotalPrice float64     `json:"total_price"`
	Currency   string      `json:"currency" gorm:"default:USD"`
	Items      []OrderItem `json:"items" gorm:"foreignKey:OrderID"`

	// Shipping details
	ShippingAddress Address `json:"shipping_address" gorm:"embedded;embeddedPrefix:shipping_"`
	BillingAddress  Address `json:"billing_address" gorm:"embedded;embeddedPrefix:billing_"`

	// Payment info
	PaymentMethod string     `json:"payment_method"`
	PaymentID     string     `json:"payment_id"`
	PaidAt        *time.Time `json:"paid_at"`

	// Tracking
	TrackingNumber string     `json:"tracking_number"`
	ShippedAt      *time.Time `json:"shipped_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`

	// Metadata
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrderItem struct {
	ID        string  `json:"id" gorm:"primaryKey"`
	OrderID   string  `json:"order_id" gorm:"not null;index"`
	ProductID string  `json:"product_id" gorm:"not null"`
	Quantity  int     `json:"quantity" gorm:"not null"`
	Price     float64 `json:"price" gorm:"not null"`
	Total     float64 `json:"total"`

	// Product snapshot
	ProductName string `json:"product_name"`
	ProductSKU  string `json:"product_sku"`
}

type Address struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	Country    string `json:"country"`
	PostalCode string `json:"postal_code"`
	Phone      string `json:"phone"`
}

func (o *Order) BeforeCreate(tx *gorm.DB) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.CreatedAt = time.Now()
	o.UpdatedAt = time.Now()
	return nil
}

func (o *Order) BeforeUpdate(tx *gorm.DB) error {
	o.UpdatedAt = time.Now()
	return nil
}

func (oi *OrderItem) BeforeCreate(tx *gorm.DB) error {
	if oi.ID == "" {
		oi.ID = uuid.New().String()
	}
	oi.Total = float64(oi.Quantity) * oi.Price
	return nil
}

func (o *Order) CalculateTotal() {
	total := 0.0
	for _, item := range o.Items {
		total += item.Total
	}
	o.TotalPrice = total
}

func (o *Order) CanCancel() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusConfirmed
}

func (o *Order) CanRefund() bool {
	return o.Status == OrderStatusDelivered && o.PaidAt != nil
}