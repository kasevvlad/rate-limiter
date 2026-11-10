package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestTokenBucketAllowsUpToLimit(t *testing.T) {
	clock := newFakeClock()
	tb := NewTokenBucket(3, time.Second)
	tb.now = clock.Now

	for i := 0; i < 3; i++ {
		result, _ := tb.Allow(context.Background(), "user")
		if !result.Allowed {
			t.Fatalf("request %d was rejected", i+1)
		}
		if result.Remaining != 2-i {
			t.Errorf("request %d: remaining %d, want %d", i+1, result.Remaining, 2-i)
		}
	}

	result, _ := tb.Allow(context.Background(), "user")
	if result.Allowed {
		t.Fatal("4th request was allowed")
	}
	if result.RetryAfter != time.Second/3 {
		t.Errorf("retry after %v, want %v", result.RetryAfter, time.Second/3)
	}
}

func TestTokenBucketRefills(t *testing.T) {
	clock := newFakeClock()
	tb := NewTokenBucket(2, time.Second)
	tb.now = clock.Now

	tb.Allow(context.Background(), "user")
	tb.Allow(context.Background(), "user")

	result, _ := tb.Allow(context.Background(), "user")
	if result.Allowed {
		t.Fatal("request was allowed with empty bucket")
	}

	clock.Advance(500 * time.Millisecond)

	result, _ = tb.Allow(context.Background(), "user")
	if !result.Allowed {
		t.Fatal("request was rejected after one token refilled")
	}

	clock.Advance(time.Hour)

	result, _ = tb.Allow(context.Background(), "user")
	if result.Remaining != 1 {
		t.Errorf("remaining %d after long pause, want 1 because bucket is capped", result.Remaining)
	}
}

func TestTokenBucketKeysAreSeparate(t *testing.T) {
	tb := NewTokenBucket(1, time.Minute)

	first, _ := tb.Allow(context.Background(), "alice")
	second, _ := tb.Allow(context.Background(), "bob")

	if !first.Allowed || !second.Allowed {
		t.Fatal("different keys should have their own buckets")
	}
}

func TestTokenBucketConcurrentRequests(t *testing.T) {
	clock := newFakeClock()
	tb := NewTokenBucket(50, time.Minute)
	tb.now = clock.Now

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _ := tb.Allow(context.Background(), "user")
			if result.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 50 {
		t.Errorf("allowed %d requests, want 50", allowed)
	}
}
