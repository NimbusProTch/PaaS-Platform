package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type App struct {
	DB    *sql.DB
	Redis *redis.Client
}

type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

type Health struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

func main() {
	app := &App{}

	// Initialize PostgreSQL
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "testapp")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	app.DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: Could not connect to database: %v", err)
	} else {
		// Create table if not exists
		createTable := `
		CREATE TABLE IF NOT EXISTS todos (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			completed BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT NOW()
		)`
		app.DB.Exec(createTable)
	}

	// Initialize Redis
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")

	app.Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := app.Redis.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Could not connect to Redis: %v", err)
	}

	// Setup routes
	router := mux.NewRouter()

	// Health check
	router.HandleFunc("/health", app.HealthHandler).Methods("GET")
	router.HandleFunc("/", app.HomeHandler).Methods("GET")

	// Todo endpoints
	router.HandleFunc("/api/todos", app.GetTodos).Methods("GET")
	router.HandleFunc("/api/todos", app.CreateTodo).Methods("POST")
	router.HandleFunc("/api/todos/{id}", app.UpdateTodo).Methods("PUT")
	router.HandleFunc("/api/todos/{id}", app.DeleteTodo).Methods("DELETE")

	// Cache endpoint
	router.HandleFunc("/api/cache/{key}", app.GetCache).Methods("GET")
	router.HandleFunc("/api/cache/{key}", app.SetCache).Methods("POST")

	// Static files
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func (app *App) HomeHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"message": "InfraForge Test Application",
		"version": getEnv("VERSION", "v1.0.0"),
		"environment": getEnv("ENVIRONMENT", "development"),
	}
	json.NewEncoder(w).Encode(response)
}

func (app *App) HealthHandler(w http.ResponseWriter, r *http.Request) {
	health := Health{
		Status:   "healthy",
		Database: "disconnected",
		Redis:    "disconnected",
	}

	// Check database
	if app.DB != nil {
		if err := app.DB.Ping(); err == nil {
			health.Database = "connected"
		}
	}

	// Check Redis
	if app.Redis != nil {
		ctx := context.Background()
		if err := app.Redis.Ping(ctx).Err(); err == nil {
			health.Redis = "connected"
		}
	}

	// Set status code based on health
	if health.Database == "disconnected" || health.Redis == "disconnected" {
		health.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (app *App) GetTodos(w http.ResponseWriter, r *http.Request) {
	if app.DB == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	todos := []Todo{}
	rows, err := app.DB.Query("SELECT id, title, completed, created_at FROM todos ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var todo Todo
		err := rows.Scan(&todo.ID, &todo.Title, &todo.Completed, &todo.CreatedAt)
		if err != nil {
			continue
		}
		todos = append(todos, todo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todos)
}

func (app *App) CreateTodo(w http.ResponseWriter, r *http.Request) {
	if app.DB == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	var todo Todo
	if err := json.NewDecoder(r.Body).Decode(&todo); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := app.DB.QueryRow(
		"INSERT INTO todos (title, completed) VALUES ($1, $2) RETURNING id, created_at",
		todo.Title, todo.Completed,
	).Scan(&todo.ID, &todo.CreatedAt)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache in Redis
	if app.Redis != nil {
		ctx := context.Background()
		key := fmt.Sprintf("todo:%d", todo.ID)
		data, _ := json.Marshal(todo)
		app.Redis.Set(ctx, key, string(data), 5*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(todo)
}

func (app *App) UpdateTodo(w http.ResponseWriter, r *http.Request) {
	if app.DB == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var todo Todo
	if err := json.NewDecoder(r.Body).Decode(&todo); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err := app.DB.Exec(
		"UPDATE todos SET title = $1, completed = $2 WHERE id = $3",
		todo.Title, todo.Completed, id,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache
	if app.Redis != nil {
		ctx := context.Background()
		key := fmt.Sprintf("todo:%s", id)
		app.Redis.Del(ctx, key)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func (app *App) DeleteTodo(w http.ResponseWriter, r *http.Request) {
	if app.DB == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	_, err := app.DB.Exec("DELETE FROM todos WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clear cache
	if app.Redis != nil {
		ctx := context.Background()
		key := fmt.Sprintf("todo:%s", id)
		app.Redis.Del(ctx, key)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (app *App) GetCache(w http.ResponseWriter, r *http.Request) {
	if app.Redis == nil {
		http.Error(w, "Redis not connected", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	key := vars["key"]

	ctx := context.Background()
	value, err := app.Redis.Get(ctx, key).Result()
	if err == redis.Nil {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"key":   key,
		"value": value,
	})
}

func (app *App) SetCache(w http.ResponseWriter, r *http.Request) {
	if app.Redis == nil {
		http.Error(w, "Redis not connected", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	key := vars["key"]

	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	err := app.Redis.Set(ctx, key, data["value"], 5*time.Minute).Err()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cached",
		"key":    key,
		"ttl":    "5m",
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}