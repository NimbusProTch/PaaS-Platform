package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/infraforge/order-service/internal/models"
	"github.com/infraforge/order-service/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("order-handler")

type OrderHandler struct {
	service service.OrderService
}

func NewOrderHandler(service service.OrderService) *OrderHandler {
	return &OrderHandler{
		service: service,
	}
}

// CreateOrder handles POST /api/v1/orders
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "CreateOrder")
	defer span.End()

	var order models.Order
	if err := c.ShouldBindJSON(&order); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context (would be set by auth middleware)
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-ID") // For testing
	}
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	order.UserID = userID

	if err := h.service.CreateOrder(ctx, &order); err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	span.SetAttributes(attribute.String("order.id", order.ID))
	c.JSON(http.StatusCreated, order)
}

// GetOrder handles GET /api/v1/orders/:id
func (h *OrderHandler) GetOrder(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "GetOrder")
	defer span.End()

	orderID := c.Param("id")
	span.SetAttributes(attribute.String("order.id", orderID))

	order, err := h.service.GetOrder(ctx, orderID)
	if err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	c.JSON(http.StatusOK, order)
}

// GetUserOrders handles GET /api/v1/users/:userId/orders
func (h *OrderHandler) GetUserOrders(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "GetUserOrders")
	defer span.End()

	userID := c.Param("userId")
	if userID == "" {
		userID = c.GetString("user_id")
	}

	span.SetAttributes(attribute.String("user.id", userID))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	orders, total, err := h.service.GetUserOrders(ctx, userID, page, limit)
	if err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"page":   page,
		"limit":  limit,
		"total":  total,
	})
}

// UpdateOrder handles PUT /api/v1/orders/:id
func (h *OrderHandler) UpdateOrder(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "UpdateOrder")
	defer span.End()

	orderID := c.Param("id")
	span.SetAttributes(attribute.String("order.id", orderID))

	var order models.Order
	if err := c.ShouldBindJSON(&order); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order.ID = orderID

	if err := h.service.UpdateOrder(ctx, &order); err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, order)
}

// UpdateOrderStatus handles PATCH /api/v1/orders/:id/status
func (h *OrderHandler) UpdateOrderStatus(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "UpdateOrderStatus")
	defer span.End()

	orderID := c.Param("id")
	span.SetAttributes(attribute.String("order.id", orderID))

	var req struct {
		Status models.OrderStatus `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateOrderStatus(ctx, orderID, req.Status); err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "order status updated"})
}

// CancelOrder handles DELETE /api/v1/orders/:id
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "CancelOrder")
	defer span.End()

	orderID := c.Param("id")
	span.SetAttributes(attribute.String("order.id", orderID))

	if err := h.service.CancelOrder(ctx, orderID); err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "order cancelled"})
}

// GetOrdersByStatus handles GET /api/v1/orders/status/:status
func (h *OrderHandler) GetOrdersByStatus(c *gin.Context) {
	ctx, span := tracer.Start(c.Request.Context(), "GetOrdersByStatus")
	defer span.End()

	status := models.OrderStatus(c.Param("status"))
	span.SetAttributes(attribute.String("order.status", string(status)))

	// This would typically be an admin-only endpoint
	// Check for admin role here

	orders, err := h.service.GetOrdersByStatus(ctx, status)
	if err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

// CalculateOrderTotal handles POST /api/v1/orders/calculate
func (h *OrderHandler) CalculateOrderTotal(c *gin.Context) {
	_, span := tracer.Start(c.Request.Context(), "CalculateOrderTotal")
	defer span.End()

	var order models.Order
	if err := c.ShouldBindJSON(&order); err != nil {
		span.RecordError(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	total := h.service.CalculateOrderTotal(&order)

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"currency": order.Currency,
		"items":    len(order.Items),
	})
}

// Health handles GET /health
func (h *OrderHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// Ready handles GET /ready
func (h *OrderHandler) Ready(c *gin.Context) {
	// Check database and other dependencies
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}