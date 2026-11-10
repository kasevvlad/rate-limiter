package ratelimit

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type SlidingWindow struct {
	client *redis.Client
	limit  int
	window time.Duration
	prefix string
	now    func() time.Time
}

func NewSlidingWindow(client *redis.Client, limit int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		client: client,
		limit:  limit,
		window: window,
		prefix: "ratelimit:",
		now:    time.Now,
	}
}

func (sw *SlidingWindow) Allow(ctx context.Context, key string) (Result, error) {
	now := sw.now()
	redisKey := sw.prefix + key
	windowStart := now.Add(-sw.window)
	member := fmt.Sprintf("%d-%d", now.UnixNano(), rand.Int64())

	var count *redis.IntCmd
	_, err := sw.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, redisKey, "-inf", strconv.FormatInt(windowStart.UnixMicro(), 10))
		pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now.UnixMicro()), Member: member})
		count = pipe.ZCard(ctx, redisKey)
		pipe.PExpire(ctx, redisKey, sw.window)
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	used := int(count.Val())
	if used <= sw.limit {
		return Result{Allowed: true, Limit: sw.limit, Remaining: sw.limit - used}, nil
	}

	if err := sw.client.ZRem(ctx, redisKey, member).Err(); err != nil {
		return Result{}, err
	}

	oldest, err := sw.client.ZRangeWithScores(ctx, redisKey, 0, 0).Result()
	if err != nil {
		return Result{}, err
	}

	retryAfter := sw.window
	if len(oldest) > 0 {
		oldestTime := time.UnixMicro(int64(oldest[0].Score))
		retryAfter = oldestTime.Add(sw.window).Sub(now)
	}

	return Result{Allowed: false, Limit: sw.limit, Remaining: 0, RetryAfter: retryAfter}, nil
}
