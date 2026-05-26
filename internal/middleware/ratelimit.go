package middleware

import (
	"sync"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limit    rate.Limit
	burst    int
	limiters map[int64]*rate.Limiter
	mu       sync.Mutex
}

func NewRateLimiter(msgPerSec int) *RateLimiter {
	if msgPerSec <= 0 {
		return nil
	}
	return &RateLimiter{
		limit:    rate.Limit(msgPerSec),
		burst:    msgPerSec,
		limiters: make(map[int64]*rate.Limiter),
	}
}

func (rl *RateLimiter) Allow(qq int64) bool {
	if rl == nil {
		return true
	}
	rl.mu.Lock()
	lim, ok := rl.limiters[qq]
	if !ok {
		lim = rate.NewLimiter(rl.limit, rl.burst)
		rl.limiters[qq] = lim
	}
	rl.mu.Unlock()
	return lim.Allow()
}

func (rl *RateLimiter) Remove(qq int64) {
	if rl == nil {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limiters, qq)
}
