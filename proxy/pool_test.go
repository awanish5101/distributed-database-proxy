package proxy

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	if cb.State() != StateClosed {
		t.Fatalf("expected state CLOSED, got %s", cb.State())
	}

	// 1st failure
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Fatalf("expected state CLOSED after 1 failure, got %s", cb.State())
	}

	// 2nd failure trips breaker
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected state OPEN after 2 failures, got %s", cb.State())
	}

	// Should reject executions while open
	if err := cb.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Sleep past reset timeout
	time.Sleep(60 * time.Millisecond)

	// Next call transitions to Half-Open and allows 1 probe
	if err := cb.Allow(); err != nil {
		t.Fatalf("expected probe allowed, got %v", err)
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected state HALF_OPEN, got %s", cb.State())
	}

	// Probe succeeds -> Closed
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Fatalf("expected state CLOSED after successful probe, got %s", cb.State())
	}
}

func TestBackendPool_AcquireAndRelease(t *testing.T) {
	pool := NewBackendPool("postgres-db1:5432", 2)

	if err := pool.Acquire(); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}
	if err := pool.Acquire(); err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}

	// 3rd acquire exceeds capacity
	if err := pool.Acquire(); err == nil {
		t.Fatalf("expected pool exhaustion error, got nil")
	}

	// Release 1 and acquire again
	pool.Release(nil)
	if err := pool.Acquire(); err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
}
