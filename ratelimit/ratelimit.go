package ratelimit

import (
	"sync"
	"time"
)

// Limiter is the interface for query rate limiters.
type Limiter interface {
	Allow(key string) bool
}

// TokenBucketLimiter implements a thread-safe token bucket rate limiter per key.
type TokenBucketLimiter struct {
	mu       sync.Mutex
	capacity float64
	rate     float64 // Tokens per second
	buckets  map[string]*bucket
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucketLimiter creates a new TokenBucketLimiter.
// capacity: max burst tokens; rate: tokens replenished per second.
func NewTokenBucketLimiter(capacity, rate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		capacity: capacity,
		rate:     rate,
		buckets:  make(map[string]*bucket),
	}
}

// Allow checks if a request with the given key is permitted under the rate limit.
func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[key]
	if !exists {
		l.buckets[key] = &bucket{
			tokens:     l.capacity - 1.0,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}
