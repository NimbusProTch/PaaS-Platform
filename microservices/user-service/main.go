package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// User model
type User struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"unique;not null"`
	Email     string    `json:"email" gorm:"unique;not null"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone"`
	Active    bool      `json:"active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserService handles user operations
type UserService struct {
	db          *gorm.DB
	redis       *redis.Client
	rabbitCh    *amqp.Channel
	tracer      string
	httpCounter *prometheus.CounterVec
	dbLatency   *prometheus.HistogramVec
}

var (
	// Prometheus metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "HTTP request duration in seconds",
		},
		[]string{"method", "endpoint"},
	)

	dbOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "db_operation_duration_seconds",
			Help: "Database operation duration in seconds",
		},
		[]string{"operation", "table"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpDuration)
	prometheus.MustRegister(dbOperationDuration)
}

func main() {
	// Initialize OpenTelemetry
	initTracer()

	// Connect to PostgreSQL
	db := connectDB()

	// Connect to Redis
	redisClient := connectRedis()

	// Connect to RabbitMQ
	rabbitConn, rabbitCh := connectRabbitMQ()
	defer rabbitConn.Close()
	defer rabbitCh.Close()

	// Create service
	service := &UserService{
		db:          db,
		redis:       redisClient,
		rabbitCh:    rabbitCh,
		tracer:      "user-service",
		httpCounter: httpRequestsTotal,
		dbLatency:   dbOperationDuration,
	}

	// Run migrations
	db.AutoMigrate(&User{})

	// Setup Gin router
	router := gin.Default()

	// Health checks
	router.GET("/health", health)
	router.GET("/ready", ready(service))

	// Metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// User endpoints
	api := router.Group("/api/v1")
	{
		api.GET("/users", service.ListUsers)
		api.GET("/users/:id", service.GetUser)
		api.POST("/users", service.CreateUser)
		api.PUT("/users/:id", service.UpdateUser)
		api.DELETE("/users/:id", service.DeleteUser)
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("User Service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func connectDB() *gorm.DB {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "users_db"),
		getEnv("DB_PORT", "5432"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return db
}

func connectRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_HOST", "localhost") + ":" + getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})
}

func connectRabbitMQ() (*amqp.Connection, *amqp.Channel) {
	url := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		getEnv("RABBITMQ_USER", "admin"),
		getEnv("RABBITMQ_PASSWORD", "rabbitmq123"),
		getEnv("RABBITMQ_HOST", "localhost"),
		getEnv("RABBITMQ_PORT", "5672"),
	)

	conn, err := amqp.Dial(url)
	if err != nil {
		log.Printf("Failed to connect to RabbitMQ: %v", err)
		return nil, nil
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Failed to open channel: %v", err)
		return conn, nil
	}

	// Declare exchange
	ch.ExchangeDeclare(
		"user-events", // name
		"topic",       // type
		true,          // durable
		false,         // auto-deleted
		false,         // internal
		false,         // no-wait
		nil,           // arguments
	)

	return conn, ch
}

func initTracer() {
	ctx := context.Background()

	// Create OTLP exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Printf("Failed to create OTLP exporter: %v", err)
		return
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("user-service"),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("environment", getEnv("ENVIRONMENT", "development")),
		)),
	)

	otel.SetTracerProvider(tp)
}

// CRUD Operations

func (s *UserService) ListUsers(c *gin.Context) {
	ctx, span := otel.Tracer(s.tracer).Start(c.Request.Context(), "ListUsers")
	defer span.End()

	timer := prometheus.NewTimer(s.dbLatency.WithLabelValues("select", "users"))
	defer timer.ObserveDuration()

	// Check cache
	cacheKey := "users:all"
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		span.SetAttributes(attribute.Bool("cache_hit", true))
		var users []User
		json.Unmarshal([]byte(cached), &users)
		c.JSON(200, users)
		return
	}

	var users []User
	if err := s.db.Find(&users).Error; err != nil {
		span.RecordError(err)
		c.JSON(500, gin.H{"error": "Failed to fetch users"})
		return
	}

	// Cache result
	data, _ := json.Marshal(users)
	s.redis.Set(ctx, cacheKey, data, time.Minute*5)

	s.httpCounter.WithLabelValues("GET", "/users", "200").Inc()
	c.JSON(200, users)
}

func (s *UserService) GetUser(c *gin.Context) {
	ctx, span := otel.Tracer(s.tracer).Start(c.Request.Context(), "GetUser")
	defer span.End()

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))

	// Check cache
	cacheKey := fmt.Sprintf("user:%s", id)
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		span.SetAttributes(attribute.Bool("cache_hit", true))
		var user User
		json.Unmarshal([]byte(cached), &user)
		c.JSON(200, user)
		return
	}

	var user User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"error": "User not found"})
			return
		}
		span.RecordError(err)
		c.JSON(500, gin.H{"error": "Failed to fetch user"})
		return
	}

	// Cache result
	data, _ := json.Marshal(user)
	s.redis.Set(ctx, cacheKey, data, time.Minute*10)

	c.JSON(200, user)
}

func (s *UserService) CreateUser(c *gin.Context) {
	ctx, span := otel.Tracer(s.tracer).Start(c.Request.Context(), "CreateUser")
	defer span.End()

	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user.ID = uuid.New().String()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	timer := prometheus.NewTimer(s.dbLatency.WithLabelValues("insert", "users"))
	defer timer.ObserveDuration()

	if err := s.db.Create(&user).Error; err != nil {
		span.RecordError(err)
		c.JSON(500, gin.H{"error": "Failed to create user"})
		return
	}

	// Publish event
	s.publishEvent("user.created", user)

	// Invalidate cache
	s.redis.Del(ctx, "users:all")

	s.httpCounter.WithLabelValues("POST", "/users", "201").Inc()
	c.JSON(201, user)
}

func (s *UserService) UpdateUser(c *gin.Context) {
	ctx, span := otel.Tracer(s.tracer).Start(c.Request.Context(), "UpdateUser")
	defer span.End()

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))

	var updates User
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	updates.UpdatedAt = time.Now()

	timer := prometheus.NewTimer(s.dbLatency.WithLabelValues("update", "users"))
	defer timer.ObserveDuration()

	result := s.db.Model(&User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		span.RecordError(result.Error)
		c.JSON(500, gin.H{"error": "Failed to update user"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	// Publish event
	updates.ID = id
	s.publishEvent("user.updated", updates)

	// Invalidate cache
	s.redis.Del(ctx, "users:all", fmt.Sprintf("user:%s", id))

	c.JSON(200, gin.H{"message": "User updated successfully"})
}

func (s *UserService) DeleteUser(c *gin.Context) {
	ctx, span := otel.Tracer(s.tracer).Start(c.Request.Context(), "DeleteUser")
	defer span.End()

	id := c.Param("id")
	span.SetAttributes(attribute.String("user.id", id))

	timer := prometheus.NewTimer(s.dbLatency.WithLabelValues("delete", "users"))
	defer timer.ObserveDuration()

	result := s.db.Delete(&User{}, "id = ?", id)
	if result.Error != nil {
		span.RecordError(result.Error)
		c.JSON(500, gin.H{"error": "Failed to delete user"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	// Publish event
	s.publishEvent("user.deleted", gin.H{"id": id})

	// Invalidate cache
	s.redis.Del(ctx, "users:all", fmt.Sprintf("user:%s", id))

	c.JSON(200, gin.H{"message": "User deleted successfully"})
}

func (s *UserService) publishEvent(eventType string, data interface{}) {
	if s.rabbitCh == nil {
		return
	}

	body, _ := json.Marshal(gin.H{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().Unix(),
		"service":   "user-service",
	})

	err := s.rabbitCh.Publish(
		"user-events", // exchange
		eventType,     // routing key
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})

	if err != nil {
		log.Printf("Failed to publish event: %v", err)
	}
}

func health(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

func ready(s *UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check DB
		sqlDB, _ := s.db.DB()
		if err := sqlDB.Ping(); err != nil {
			c.JSON(503, gin.H{"status": "not ready", "error": "database not reachable"})
			return
		}

		// Check Redis
		if err := s.redis.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(503, gin.H{"status": "not ready", "error": "redis not reachable"})
			return
		}

		c.JSON(200, gin.H{"status": "ready"})
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}