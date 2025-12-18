package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

type InfoResponse struct {
	AppName     string            `json:"app_name"`
	Version     string            `json:"version"`
	Environment map[string]string `json:"environment"`
	Timestamp   time.Time         `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   getEnv("APP_VERSION", "v1.0.0"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	response := InfoResponse{
		AppName: "backend-service",
		Version: getEnv("APP_VERSION", "v1.0.0"),
		Environment: map[string]string{
			"database_url": getEnv("DATABASE_URL", "not_configured"),
			"redis_url":    getEnv("REDIS_URL", "not_configured"),
			"environment":  getEnv("ENVIRONMENT", "development"),
		},
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Backend Service API",
		"status":  "running",
		"version": getEnv("APP_VERSION", "v1.0.0"),
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	port := getEnv("PORT", "8080")

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/info", infoHandler)

	log.Printf("Backend service starting on port %s", port)
	log.Printf("Version: %s", getEnv("APP_VERSION", "v1.0.0"))

	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}