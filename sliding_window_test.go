package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newSlidingWindow(t *testing.T, limit int, window time.Duration) (*SlidingWindow, *fakeClock, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { client.Close() })

	clock := newFakeClock()
	sw := NewSlidingWindow(client, limit, window)
	sw.now = clock.Now

	return sw, clock, server
}

func TestSlidingWindowAllowsUpToLimit(t *testing.T) {
	sw, clock, _ := newSlidingWindow(t, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		result, err := sw.Allow(ctx, "user")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Allowed {
			t.Fatalf("request %d was rejected", i+1)
		}
		if result.Remaining != 2-i {
			t.Errorf("request %d: remaining %d, want %d", i+1, result.Remaining, 2-i)
		}
		clock.Advance(10 * time.Second)
	}

	result, _ := sw.Allow(ctx, "user")
	if result.Allowed {
		t.Fatal("4th request was allowed")
	}
	if result.RetryAfter != 30*time.Second {
		t.Errorf("retry after %v, want 30s until the first request leaves the window", result.RetryAfter)
	}
}

func TestSlidingWindowOldRequestsLeaveTheWindow(t *testing.T) {
	sw, clock, _ := newSlidingWindow(t, 2, time.Minute)
	ctx := context.Background()

	sw.Allow(ctx, "user")
	clock.Advance(30 * time.Second)
	sw.Allow(ctx, "user")

	result, _ := sw.Allow(ctx, "user")
	if result.Allowed {
		t.Fatal("request was allowed over the limit")
	}

	clock.Advance(31 * time.Second)

	result, _ = sw.Allow(ctx, "user")
	if !result.Allowed {
		t.Fatal("request was rejected after the first one left the window")
	}
}

func TestSlidingWindowRejectedRequestsAreNotCounted(t *testing.T) {
	sw, _, server := newSlidingWindow(t, 2, time.Minute)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		sw.Allow(ctx, "user")
	}

	members, err := server.ZMembers("ratelimit:user")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Errorf("redis has %d entries, want 2", len(members))
	}
}

func TestSlidingWindowSetsExpiration(t *testing.T) {
	sw, _, server := newSlidingWindow(t, 5, time.Minute)

	sw.Allow(context.Background(), "user")

	if ttl := server.TTL("ratelimit:user"); ttl != time.Minute {
		t.Errorf("ttl is %v, want 1m", ttl)
	}
}

func TestSlidingWindowRedisDown(t *testing.T) {
	sw, _, server := newSlidingWindow(t, 5, time.Minute)
	server.Close()

	if _, err := sw.Allow(context.Background(), "user"); err == nil {
		t.Fatal("expected an error when redis is down")
	}
}
