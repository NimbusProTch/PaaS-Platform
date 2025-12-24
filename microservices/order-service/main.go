package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/infraforge/order-service/internal/handlers"
	"github.com/infraforge/order-service/internal/models"
	"github.com/infraforge/order-service/internal/repository"
	"github.com/infraforge/order-service/internal/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Duration of HTTP requests in seconds",
		},
		[]string{"method", "route", "status_code"},
	)

	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "db_query_duration_seconds",
			Help: "Duration of database queries in seconds",
		},
		[]string{"operation", "table"},
	)

	orderTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "orders_total",
			Help: "Total number of orders",
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(dbQueryDuration)
	prometheus.MustRegister(orderTotal)
}

func main() {
	// Configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "order_user")
	dbPass := getEnv("DB_PASSWORD", "order_pass")
	dbName := getEnv("DB_NAME", "order_db")

	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPass := getEnv("REDIS_PASSWORD", "")

	amqpURL := getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")

	jaegerEndpoint := getEnv("JAEGER_ENDPOINT", "localhost:4317")

	port := getEnv("PORT", "8082")
	environment := getEnv("ENVIRONMENT", "development")

	// Initialize OpenTelemetry
	tp, err := initTracer(jaegerEndpoint)
	if err != nil {
		log.Printf("Failed to initialize tracer: %v", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Printf("Error shutting down tracer provider: %v", err)
			}
		}()
	}

	// Database connection
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		dbHost, dbUser, dbPass, dbName, dbPort)

	gormConfig := &gorm.Config{}
	if environment == "development" {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate models
	if err := db.AutoMigrate(&models.Order{}, &models.OrderItem{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Redis connection
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPass,
		DB:       0,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		// Continue without Redis (caching will be disabled)
		rdb = nil
	}

	// RabbitMQ connection
	var amqpChan *amqp.Channel
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		log.Printf("Failed to connect to RabbitMQ: %v", err)
	} else {
		defer conn.Close()

		amqpChan, err = conn.Channel()
		if err != nil {
			log.Printf("Failed to open RabbitMQ channel: %v", err)
		} else {
			defer amqpChan.Close()

			// Declare exchange
			err = amqpChan.ExchangeDeclare(
				"orders", // name
				"topic",  // type
				true,     // durable
				false,    // auto-deleted
				false,    // internal
				false,    // no-wait
				nil,      // arguments
			)
			if err != nil {
				log.Printf("Failed to declare exchange: %v", err)
			}
		}
	}

	// Initialize repository and service
	orderRepo := repository.NewOrderRepository(db, rdb)
	orderService := service.NewOrderService(orderRepo, amqpChan)
	orderHandler := handlers.NewOrderHandler(orderService)

	// Setup Gin router
	if environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(prometheusMiddleware())
	router.Use(corsMiddleware())
	router.Use(tracingMiddleware())

	// Health checks
	router.GET("/health", orderHandler.Health)
	router.GET("/ready", orderHandler.Ready)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API routes
	v1 := router.Group("/api/v1")
	{
		// Order routes
		v1.POST("/orders", orderHandler.CreateOrder)
		v1.GET("/orders/:id", orderHandler.GetOrder)
		v1.PUT("/orders/:id", orderHandler.UpdateOrder)
		v1.DELETE("/orders/:id", orderHandler.CancelOrder)
		v1.PATCH("/orders/:id/status", orderHandler.UpdateOrderStatus)
		v1.POST("/orders/calculate", orderHandler.CalculateOrderTotal)

		// User orders
		v1.GET("/users/:userId/orders", orderHandler.GetUserOrders)

		// Admin routes
		v1.GET("/orders/status/:status", orderHandler.GetOrdersByStatus)
	}

	// Start background workers
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := orderService.ProcessPendingOrders(context.Background()); err != nil {
					log.Printf("Error processing pending orders: %v", err)
				}
			}
		}
	}()

	// Start server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Order Service started on port %s", port)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

func initTracer(endpoint string) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(
		context.Background(),
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("order-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		httpRequestDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			status,
		).Observe(duration)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func tracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tracer := otel.Tracer("gin-middleware")
		ctx, span := tracer.Start(c.Request.Context(), c.Request.URL.Path)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}