package middleware

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5)
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}

	qq := int64(10001)
	for i := 0; i < 5; i++ {
		if !rl.Allow(qq) {
			t.Fatalf("expected Allow()=true for message %d", i+1)
		}
	}

	if rl.Allow(qq) {
		t.Fatal("expected Allow()=false after burst exhausted")
	}
}

func TestRateLimiterPerUser(t *testing.T) {
	rl := NewRateLimiter(2)

	qq1 := int64(10001)
	qq2 := int64(10002)

	if !rl.Allow(qq1) {
		t.Fatal("qq1 first message should be allowed")
	}
	if !rl.Allow(qq1) {
		t.Fatal("qq1 second message should be allowed")
	}
	if rl.Allow(qq1) {
		t.Fatal("qq1 third message should be blocked")
	}

	if !rl.Allow(qq2) {
		t.Fatal("qq2 first message should be allowed (independent limiter)")
	}
}

func TestRateLimiterRemove(t *testing.T) {
	rl := NewRateLimiter(1)
	qq := int64(10001)

	rl.Allow(qq)
	if rl.Allow(qq) {
		t.Fatal("should be blocked after burst")
	}

	rl.Remove(qq)

	if !rl.Allow(qq) {
		t.Fatal("should be allowed after remove (new limiter)")
	}
}

func TestRateLimiterNil(t *testing.T) {
	var rl *RateLimiter

	if !rl.Allow(10001) {
		t.Fatal("nil rate limiter should always allow")
	}

	rl.Remove(10001)
}

func TestRateLimiterZero(t *testing.T) {
	rl := NewRateLimiter(0)
	if rl != nil {
		t.Fatal("expected nil rate limiter for msgPerSec=0")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(10)
	qq := int64(10001)

	for i := 0; i < 10; i++ {
		if !rl.Allow(qq) {
			t.Fatalf("message %d should be allowed", i+1)
		}
	}
	if rl.Allow(qq) {
		t.Fatal("should be blocked after burst")
	}

	time.Sleep(200 * time.Millisecond)

	if !rl.Allow(qq) {
		t.Fatal("should be allowed after token refill")
	}
}
