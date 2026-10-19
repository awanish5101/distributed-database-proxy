package proxy

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a backend circuit breaker.
type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

// CircuitBreaker guards backends from cascading failures.
type CircuitBreaker struct {
	mu             sync.Mutex
	state          CircuitState
	failCount      int
	failThreshold  int
	resetTimeout   time.Duration
	lastStateShift time.Time
}

// NewCircuitBreaker creates a circuit breaker with threshold and reset timeout.
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:          StateClosed,
		failThreshold:  threshold,
		resetTimeout:   timeout,
		lastStateShift: time.Now(),
	}
}

var ErrCircuitOpen = errors.New("circuit breaker is open; backend unreachable")

// Allow verifies if an execution should be permitted.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	if cb.state == StateOpen {
		if now.Sub(cb.lastStateShift) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.lastStateShift = now
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}

// RecordSuccess reports a successful execution.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount = 0
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.lastStateShift = time.Now()
	}
}

// RecordFailure reports a failed execution.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount++
	if cb.state == StateHalfOpen || cb.failCount >= cb.failThreshold {
		cb.state = StateOpen
		cb.lastStateShift = time.Now()
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// BackendPool tracks connections, circuit breakers, and metrics for a database instance.
type BackendPool struct {
	mu             sync.RWMutex
	Address        string
	MaxConnections int
	ActiveConns    int
	Breaker        *CircuitBreaker
}

// NewBackendPool initializes a pool wrapper for a database address.
func NewBackendPool(address string, maxConns int) *BackendPool {
	return &BackendPool{
		Address:        address,
		MaxConnections: maxConns,
		Breaker:        NewCircuitBreaker(3, 500*time.Millisecond),
	}
}

// Acquire reserves a connection from the pool if under capacity and circuit is closed.
func (bp *BackendPool) Acquire() error {
	if err := bp.Breaker.Allow(); err != nil {
		return err
	}

	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.ActiveConns >= bp.MaxConnections {
		return errors.New("connection pool exhausted")
	}
	bp.ActiveConns++
	return nil
}

// Release returns a connection to the pool.
func (bp *BackendPool) Release(err error) {
	bp.mu.Lock()
	if bp.ActiveConns > 0 {
		bp.ActiveConns--
	}
	bp.mu.Unlock()

	if err != nil {
		bp.Breaker.RecordFailure()
	} else {
		bp.Breaker.RecordSuccess()
	}
}
