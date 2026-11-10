package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

type bucket struct {
	tokens    float64
	updatedAt time.Time
}

type TokenBucket struct {
	limit   int
	perSec  float64
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

func NewTokenBucket(limit int, per time.Duration) *TokenBucket {
	return &TokenBucket{
		limit:   limit,
		perSec:  float64(limit) / per.Seconds(),
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

func (tb *TokenBucket) Allow(ctx context.Context, key string) (Result, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := tb.now()

	b, ok := tb.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(tb.limit), updatedAt: now}
		tb.buckets[key] = b
	}

	elapsed := now.Sub(b.updatedAt).Seconds()
	b.tokens = math.Min(float64(tb.limit), b.tokens+elapsed*tb.perSec)
	b.updatedAt = now

	if b.tokens >= 1 {
		b.tokens--
		return Result{Allowed: true, Limit: tb.limit, Remaining: int(b.tokens)}, nil
	}

	missing := 1 - b.tokens
	retryAfter := time.Duration(missing / tb.perSec * float64(time.Second))

	return Result{Allowed: false, Limit: tb.limit, Remaining: 0, RetryAfter: retryAfter}, nil
}
