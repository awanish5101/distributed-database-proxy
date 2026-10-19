package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketLimiter_Allow(t *testing.T) {
	// Capacity 3, rate 2 tokens/sec
	limiter := NewTokenBucketLimiter(3, 2)
	key := "tenant_rate_test"

	// Initial burst of 3 should succeed
	for i := 0; i < 3; i++ {
		if !limiter.Allow(key) {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}

	// 4th immediate request should be rejected
	if limiter.Allow(key) {
		t.Fatalf("4th request should have been rejected by rate limiter")
	}

	// Wait 600ms (should replenish at least 1 token)
	time.Sleep(600 * time.Millisecond)
	if !limiter.Allow(key) {
		t.Fatalf("request after refill should have been allowed")
	}
}

func TestTokenBucketLimiter_Concurrent(t *testing.T) {
	limiter := NewTokenBucketLimiter(50, 100)
	key := "concurrent_tenant"

	var wg sync.WaitGroup
	allowedCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow(key) {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	if allowedCount > 55 {
		t.Fatalf("burst allowed %d requests, expected <= 55", allowedCount)
	}
}
