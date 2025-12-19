package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/streadway/amqp"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Models
type NotificationType string

const (
	NotificationTypeEmail    NotificationType = "email"
	NotificationTypeSMS      NotificationType = "sms"
	NotificationTypePush     NotificationType = "push"
	NotificationTypeWebhook  NotificationType = "webhook"
	NotificationTypeInApp    NotificationType = "in_app"
)

type NotificationStatus string

const (
	NotificationStatusPending   NotificationStatus = "pending"
	NotificationStatusSent      NotificationStatus = "sent"
	NotificationStatusFailed    NotificationStatus = "failed"
	NotificationStatusDelivered NotificationStatus = "delivered"
	NotificationStatusBounced   NotificationStatus = "bounced"
)

type Notification struct {
	ID         string               `json:"id" gorm:"primaryKey"`
	UserID     string               `json:"user_id" gorm:"index"`
	Type       NotificationType     `json:"type" gorm:"type:varchar(20)"`
	Status     NotificationStatus   `json:"status" gorm:"type:varchar(20);default:pending"`
	Template   string               `json:"template"`
	Subject    string               `json:"subject"`
	Content    string               `json:"content" gorm:"type:text"`
	Recipient  string               `json:"recipient"` // email, phone number, device token
	Metadata   map[string]interface{} `json:"metadata" gorm:"serializer:json"`
	Priority   int                  `json:"priority" gorm:"default:5"` // 1-10, 1 being highest
	RetryCount int                  `json:"retry_count" gorm:"default:0"`
	MaxRetries int                  `json:"max_retries" gorm:"default:3"`

	ScheduledAt *time.Time `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
	FailedAt    *time.Time `json:"failed_at"`

	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Template struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"unique;not null"`
	Type        NotificationType `json:"type"`
	Subject     string    `json:"subject"`
	Content     string    `json:"content" gorm:"type:text"`
	Variables   []string  `json:"variables" gorm:"serializer:json"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Metrics
var (
	notificationsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total number of notifications sent",
		},
		[]string{"type", "status"},
	)

	notificationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "notification_send_duration_seconds",
			Help: "Duration of sending notifications",
		},
		[]string{"type"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Duration of HTTP requests",
		},
		[]string{"method", "route", "status_code"},
	)
)

func init() {
	prometheus.MustRegister(notificationsSent)
	prometheus.MustRegister(notificationDuration)
	prometheus.MustRegister(httpRequestDuration)
}

// Services
type EmailService struct {
	smtpHost string
	smtpPort string
	username string
	password string
	from     string
}

func NewEmailService() *EmailService {
	return &EmailService{
		smtpHost: getEnv("SMTP_HOST", "smtp.gmail.com"),
		smtpPort: getEnv("SMTP_PORT", "587"),
		username: getEnv("SMTP_USERNAME", ""),
		password: getEnv("SMTP_PASSWORD", ""),
		from:     getEnv("SMTP_FROM", "noreply@example.com"),
	}
}

func (s *EmailService) Send(to, subject, body string) error {
	if s.username == "" || s.password == "" {
		log.Printf("Email service not configured, skipping email to %s", to)
		return nil
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.smtpHost)

	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", to, subject, body))

	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)
	return smtp.SendMail(addr, auth, s.from, []string{to}, msg)
}

type SMSService struct {
	client *twilio.RestClient
	from   string
}

func NewSMSService() *SMSService {
	accountSid := getEnv("TWILIO_ACCOUNT_SID", "")
	authToken := getEnv("TWILIO_AUTH_TOKEN", "")
	from := getEnv("TWILIO_FROM_NUMBER", "")

	if accountSid == "" || authToken == "" {
		log.Println("SMS service not configured")
		return &SMSService{}
	}

	return &SMSService{
		client: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: accountSid,
			Password: authToken,
		}),
		from: from,
	}
}

func (s *SMSService) Send(to, message string) error {
	if s.client == nil {
		log.Printf("SMS service not configured, skipping SMS to %s", to)
		return nil
	}

	params := &openapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(s.from)
	params.SetBody(message)

	_, err := s.client.Api.CreateMessage(params)
	return err
}

type NotificationService struct {
	db           *gorm.DB
	redis        *redis.Client
	emailService *EmailService
	smsService   *SMSService
	amqpChan     *amqp.Channel
	tracer       trace.Tracer
}

func NewNotificationService(db *gorm.DB, redis *redis.Client, amqpChan *amqp.Channel) *NotificationService {
	return &NotificationService{
		db:           db,
		redis:        redis,
		emailService: NewEmailService(),
		smsService:   NewSMSService(),
		amqpChan:     amqpChan,
		tracer:       otel.Tracer("notification-service"),
	}
}

func (s *NotificationService) SendNotification(ctx context.Context, notification *Notification) error {
	ctx, span := s.tracer.Start(ctx, "SendNotification")
	defer span.End()

	start := time.Now()
	defer func() {
		notificationDuration.WithLabelValues(string(notification.Type)).Observe(time.Since(start).Seconds())
	}()

	// Generate ID if not set
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}

	// Save to database
	if err := s.db.Create(notification).Error; err != nil {
		return fmt.Errorf("failed to save notification: %w", err)
	}

	// Send based on type
	var err error
	switch notification.Type {
	case NotificationTypeEmail:
		err = s.emailService.Send(notification.Recipient, notification.Subject, notification.Content)
	case NotificationTypeSMS:
		err = s.smsService.Send(notification.Recipient, notification.Content)
	case NotificationTypeWebhook:
		err = s.sendWebhook(notification)
	case NotificationTypePush:
		err = s.sendPushNotification(notification)
	case NotificationTypeInApp:
		err = s.saveInAppNotification(notification)
	default:
		err = fmt.Errorf("unsupported notification type: %s", notification.Type)
	}

	// Update status
	now := time.Now()
	if err != nil {
		notification.Status = NotificationStatusFailed
		notification.Error = err.Error()
		notification.FailedAt = &now
		notification.RetryCount++

		// Schedule retry if applicable
		if notification.RetryCount < notification.MaxRetries {
			retryTime := now.Add(time.Duration(notification.RetryCount) * time.Minute)
			notification.ScheduledAt = &retryTime
		}
	} else {
		notification.Status = NotificationStatusSent
		notification.SentAt = &now
	}

	// Update in database
	s.db.Save(notification)

	// Track metrics
	notificationsSent.WithLabelValues(string(notification.Type), string(notification.Status)).Inc()

	// Publish event
	s.publishEvent("notification.sent", notification)

	return err
}

func (s *NotificationService) sendWebhook(notification *Notification) error {
	webhookURL, ok := notification.Metadata["webhook_url"].(string)
	if !ok {
		return fmt.Errorf("webhook URL not found in metadata")
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":        notification.UserID,
		"subject":        notification.Subject,
		"content":        notification.Content,
		"metadata":       notification.Metadata,
		"timestamp":      time.Now(),
	})

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *NotificationService) sendPushNotification(notification *Notification) error {
	// Implementation would depend on push service (FCM, APNS, etc.)
	log.Printf("Push notification would be sent to %s", notification.Recipient)
	return nil
}

func (s *NotificationService) saveInAppNotification(notification *Notification) error {
	// Save to Redis for quick access
	key := fmt.Sprintf("in_app:%s:%s", notification.UserID, notification.ID)
	data, _ := json.Marshal(notification)
	return s.redis.Set(context.Background(), key, data, 7*24*time.Hour).Err()
}

func (s *NotificationService) publishEvent(eventType string, notification *Notification) {
	if s.amqpChan == nil {
		return
	}

	event, _ := json.Marshal(map[string]interface{}{
		"event_type":  eventType,
		"notification": notification,
		"timestamp":   time.Now(),
	})

	s.amqpChan.Publish(
		"notifications", // exchange
		eventType,       // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        event,
		},
	)
}

// API Handlers
func createNotificationHandler(service *NotificationService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var notification Notification
		if err := c.ShouldBindJSON(&notification); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if notification.MaxRetries == 0 {
			notification.MaxRetries = 3
		}

		ctx := c.Request.Context()
		if err := service.SendNotification(ctx, &notification); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, notification)
	}
}

func getNotificationHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var notification Notification
		if err := db.First(&notification, "id = ?", id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}

		c.JSON(http.StatusOK, notification)
	}
}

func listNotificationsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		status := c.Query("status")
		notificationType := c.Query("type")
		limit := 20
		offset := 0

		query := db.Model(&Notification{})

		if userID != "" {
			query = query.Where("user_id = ?", userID)
		}
		if status != "" {
			query = query.Where("status = ?", status)
		}
		if notificationType != "" {
			query = query.Where("type = ?", notificationType)
		}

		var notifications []Notification
		query.Limit(limit).Offset(offset).Order("created_at DESC").Find(&notifications)

		var total int64
		query.Count(&total)

		c.JSON(http.StatusOK, gin.H{
			"notifications": notifications,
			"total":        total,
			"limit":        limit,
			"offset":       offset,
		})
	}
}

func retryNotificationHandler(service *NotificationService, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var notification Notification
		if err := db.First(&notification, "id = ?", id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}

		notification.RetryCount = 0
		notification.Status = NotificationStatusPending
		notification.Error = ""

		ctx := c.Request.Context()
		if err := service.SendNotification(ctx, &notification); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, notification)
	}
}

func main() {
	// Configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "notification_user")
	dbPass := getEnv("DB_PASSWORD", "notification_pass")
	dbName := getEnv("DB_NAME", "notification_db")

	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPass := getEnv("REDIS_PASSWORD", "")

	amqpURL := getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	jaegerEndpoint := getEnv("JAEGER_ENDPOINT", "localhost:4317")

	port := getEnv("PORT", "8084")
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
	if err := db.AutoMigrate(&Notification{}, &Template{}); err != nil {
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
				"notifications", // name
				"topic",        // type
				true,           // durable
				false,          // auto-deleted
				false,          // internal
				false,          // no-wait
				nil,            // arguments
			)
			if err != nil {
				log.Printf("Failed to declare exchange: %v", err)
			}
		}
	}

	// Initialize service
	notificationService := NewNotificationService(db, rdb, amqpChan)

	// Setup Gin router
	if environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(prometheusMiddleware())
	router.Use(corsMiddleware())

	// Health checks
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API routes
	v1 := router.Group("/api/v1")
	{
		v1.POST("/notifications", createNotificationHandler(notificationService))
		v1.GET("/notifications/:id", getNotificationHandler(db))
		v1.GET("/notifications", listNotificationsHandler(db))
		v1.POST("/notifications/:id/retry", retryNotificationHandler(notificationService, db))
	}

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

	log.Printf("Notification Service started on port %s", port)

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
			semconv.ServiceNameKey.String("notification-service"),
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
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}