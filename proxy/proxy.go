package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/awanish5101/distributed-database-proxy/ratelimit"
	"github.com/awanish5101/distributed-database-proxy/router"
)

// QueryRequest is the JSON payload for incoming queries.
type QueryRequest struct {
	SQL      string `json:"sql"`
	ShardKey string `json:"shard_key,omitempty"`
}

// QueryResponse contains the execution outcome and routing diagnostics.
type QueryResponse struct {
	Success        bool                  `json:"success"`
	Data           interface{}           `json:"data,omitempty"`
	Error          string                `json:"error,omitempty"`
	Routing        *router.RouteDecision `json:"routing,omitempty"`
	DurationMs     float64               `json:"duration_ms"`
	Timestamp      time.Time             `json:"timestamp"`
}

// MetricsTracker holds in-memory telemetry for requests and latency.
type MetricsTracker struct {
	mu            sync.Mutex
	TotalQueries  int64
	ReadQueries   int64
	WriteQueries  int64
	RateRejects   int64
	TotalDuration time.Duration
}

// ProxyServer is the core data infrastructure database proxy.
type ProxyServer struct {
	router  *router.Router
	limiter ratelimit.Limiter
	pools   map[string]*BackendPool
	metrics *MetricsTracker
	mu      sync.RWMutex
}

// NewProxyServer instantiates the proxy server.
func NewProxyServer(r *router.Router, lim ratelimit.Limiter) *ProxyServer {
	return &ProxyServer{
		router:  r,
		limiter: lim,
		pools:   make(map[string]*BackendPool),
		metrics: &MetricsTracker{},
	}
}

// RegisterBackendPool links an address to a connection pool.
func (ps *ProxyServer) RegisterBackendPool(address string, maxConns int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.pools[address] = NewBackendPool(address, maxConns)
}

// ExecuteQuery coordinates the full request lifecycle: rate limiting, routing, connection pooling, and telemetry.
func (ps *ProxyServer) ExecuteQuery(sql, shardKey string) (*QueryResponse, error) {
	start := time.Now()

	// 1. Shard key extraction and rate limit check
	effectiveKey := ps.router.ExtractShardKey(sql, shardKey)
	if ps.limiter != nil && !ps.limiter.Allow(effectiveKey) {
		ps.metrics.mu.Lock()
		ps.metrics.RateRejects++
		ps.metrics.mu.Unlock()
		return &QueryResponse{
			Success:    false,
			Error:      "rate limit exceeded for tenant",
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
			Timestamp:  time.Now(),
		}, errors.New("rate limit exceeded")
	}

	// 2. Query routing and read/write splitting
	decision, err := ps.router.Route(sql, shardKey)
	if err != nil {
		return &QueryResponse{
			Success:    false,
			Error:      fmt.Sprintf("routing error: %v", err),
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
			Timestamp:  time.Now(),
		}, err
	}

	// 3. Acquire connection from pool
	ps.mu.RLock()
	pool, exists := ps.pools[decision.TargetAddress]
	ps.mu.RUnlock()

	if !exists {
		// Auto-register pool for discovered nodes with default 20 connections
		ps.RegisterBackendPool(decision.TargetAddress, 20)
		ps.mu.RLock()
		pool = ps.pools[decision.TargetAddress]
		ps.mu.RUnlock()
	}

	if err := pool.Acquire(); err != nil {
		return &QueryResponse{
			Success:    false,
			Error:      fmt.Sprintf("pool acquisition failed: %v", err),
			Routing:    decision,
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
			Timestamp:  time.Now(),
		}, err
	}

	// 4. Simulate low-overhead proxy query forwarding
	// (In full production TCP mode, this forwards wire-protocol packets over unix/tcp socket)
	var execErr error
	pool.Release(execErr)

	duration := time.Since(start)
	ps.metrics.mu.Lock()
	ps.metrics.TotalQueries++
	if decision.QueryType == router.QueryTypeRead {
		ps.metrics.ReadQueries++
	} else {
		ps.metrics.WriteQueries++
	}
	ps.metrics.TotalDuration += duration
	ps.metrics.mu.Unlock()

	return &QueryResponse{
		Success:    true,
		Data:       map[string]string{"status": "executed", "records_affected": "1"},
		Routing:    decision,
		DurationMs: float64(duration.Microseconds()) / 1000.0,
		Timestamp:  time.Now(),
	}, nil
}

// ServeHTTP handles query execution via HTTP REST endpoint.
func (ps *ProxyServer) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	resp, _ := ps.ExecuteQuery(req.SQL, req.ShardKey)
	w.Header().Set("Content-Type", "application/json")
	if !resp.Success {
		w.WriteHeader(http.StatusTooManyRequests)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleHealth provides a cluster liveness and readiness probe.
func (ps *ProxyServer) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "distributed-database-proxy",
	})
}

// HandleMetrics outputs Prometheus-compatible exposition format metrics.
func (ps *ProxyServer) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	ps.metrics.mu.Lock()
	total := ps.metrics.TotalQueries
	reads := ps.metrics.ReadQueries
	writes := ps.metrics.WriteQueries
	rejects := ps.metrics.RateRejects
	ps.metrics.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	output := fmt.Sprintf(
		"# HELP db_proxy_queries_total Total SQL queries processed by proxy\n"+
			"# TYPE db_proxy_queries_total counter\n"+
			"db_proxy_queries_total{type=\"read\"} %d\n"+
			"db_proxy_queries_total{type=\"write\"} %d\n"+
			"db_proxy_queries_total{type=\"all\"} %d\n"+
			"# HELP db_proxy_rate_rejects_total Rejected queries due to rate limits\n"+
			"# TYPE db_proxy_rate_rejects_total counter\n"+
			"db_proxy_rate_rejects_total %d\n",
		reads, writes, total, rejects,
	)
	_, _ = w.Write([]byte(output))
}
