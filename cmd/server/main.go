package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	ratelimit "github.com/kasevvlad/rate-limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	addr := getEnv("ADDR", ":8080")
	store := getEnv("STORE", "memory")
	limit, err := strconv.Atoi(getEnv("LIMIT", "10"))
	if err != nil {
		log.Fatal("LIMIT must be a number")
	}
	window, err := time.ParseDuration(getEnv("WINDOW", "1m"))
	if err != nil {
		log.Fatal("WINDOW must be a duration like 30s or 1m")
	}

	var limiter ratelimit.Limiter
	switch store {
	case "memory":
		limiter = ratelimit.NewTokenBucket(limit, window)
	case "redis":
		client := redis.NewClient(&redis.Options{Addr: getEnv("REDIS_ADDR", "localhost:6379")})
		if err := client.Ping(context.Background()).Err(); err != nil {
			log.Fatal("cannot connect to redis: ", err)
		}
		limiter = ratelimit.NewSlidingWindow(client, limit, window)
	default:
		log.Fatal("STORE must be memory or redis")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /check", checkHandler(limiter))

	limited := ratelimit.Middleware(limiter, ratelimit.ByIP)
	mux.Handle("GET /hello", limited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello\n"))
	})))

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s, store %s, %d requests per %s", addr, store, limit, window)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func checkHandler(limiter ratelimit.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
			return
		}

		result, err := limiter.Allow(r.Context(), input.Key)
		if err != nil {
			log.Println("limiter error:", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		status := http.StatusOK
		if !result.Allowed {
			status = http.StatusTooManyRequests
		}

		writeJSON(w, status, map[string]any{
			"allowed":        result.Allowed,
			"limit":          result.Limit,
			"remaining":      result.Remaining,
			"retry_after_ms": result.RetryAfter.Milliseconds(),
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func getEnv(name string, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	return value
}
