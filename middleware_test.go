package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type brokenLimiter struct{}

func (brokenLimiter) Allow(ctx context.Context, key string) (Result, error) {
	return Result{}, errors.New("redis is down")
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
}

func TestMiddleware(t *testing.T) {
	handler := Middleware(NewTokenBucket(2, time.Minute), ByIP)(okHandler())

	codes := []int{}
	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:5000"
		last = httptest.NewRecorder()
		handler.ServeHTTP(last, req)
		codes = append(codes, last.Code)
	}

	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Fatalf("got codes %v, want [200 200 429]", codes)
	}
	if last.Header().Get("Retry-After") != "30" {
		t.Errorf("Retry-After is %q, want 30", last.Header().Get("Retry-After"))
	}
	if last.Header().Get("X-RateLimit-Limit") != "2" || last.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("unexpected rate limit headers %v", last.Header())
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:5000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("another ip got %d", rec.Code)
	}
}

func TestMiddlewareLetsRequestsThroughWhenLimiterFails(t *testing.T) {
	handler := Middleware(brokenLimiter{}, ByIP)(okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Errorf("got %d, want 200", rec.Code)
	}
}
