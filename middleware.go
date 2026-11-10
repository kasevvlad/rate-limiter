package ratelimit

import (
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
)

func Middleware(limiter Limiter, keyFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			result, err := limiter.Allow(r.Context(), keyFunc(r))
			if err != nil {
				log.Println("rate limiter error, letting request through:", err)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

			if !result.Allowed {
				seconds := int(math.Ceil(result.RetryAfter.Seconds()))
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
