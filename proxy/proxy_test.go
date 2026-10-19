package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awanish5101/distributed-database-proxy/ratelimit"
	"github.com/awanish5101/distributed-database-proxy/router"
)

func TestProxyServer_EndToEndExecution(t *testing.T) {
	r := router.NewRouter(50)
	r.RegisterShard(&router.ShardInfo{
		ID:       "shard-primary-region",
		Primary:  "postgres-east-master:5432",
		Replicas: []string{"postgres-east-replica:5432"},
		IsAlive:  true,
	})

	limiter := ratelimit.NewTokenBucketLimiter(10, 5)
	proxy := NewProxyServer(r, limiter)

	// Test Read query via ExecuteQuery
	resp, err := proxy.ExecuteQuery("/* shard_key: tenant_99 */ SELECT * FROM employees;", "")
	if err != nil {
		t.Fatalf("expected successful query execution, got: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected query success true, got false")
	}
	if !resp.Routing.IsReplica || resp.Routing.TargetAddress != "postgres-east-replica:5432" {
		t.Fatalf("expected routing to read replica, got: %+v", resp.Routing)
	}

	// Test Write query via ExecuteQuery
	writeResp, err := proxy.ExecuteQuery("/* shard_key: tenant_99 */ INSERT INTO employees (id, name) VALUES (1, 'Eve');", "")
	if err != nil {
		t.Fatalf("expected successful write execution, got: %v", err)
	}
	if writeResp.Routing.IsReplica || writeResp.Routing.TargetAddress != "postgres-east-master:5432" {
		t.Fatalf("expected routing to primary master, got: %+v", writeResp.Routing)
	}
}

func TestProxyServer_HTTPHandlers(t *testing.T) {
	r := router.NewRouter(50)
	r.RegisterShard(&router.ShardInfo{
		ID:       "shard-alpha",
		Primary:  "db-shard-1:5432",
		Replicas: []string{"db-shard-1-rep:5432"},
		IsAlive:  true,
	})

	limiter := ratelimit.NewTokenBucketLimiter(2, 1)
	proxy := NewProxyServer(r, limiter)

	// 1. Health check
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	proxy.HandleHealth(healthRec, healthReq)

	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected /healthz 200, got %d", healthRec.Code)
	}

	// 2. Query handler HTTP
	payload := `{"sql": "/* shard_key: corp_a */ SELECT count(*) FROM orders;"}`
	queryReq := httptest.NewRequest(http.MethodPost, "/query", bytes.NewBufferString(payload))
	queryRec := httptest.NewRecorder()
	proxy.HandleQuery(queryRec, queryReq)

	if queryRec.Code != http.StatusOK {
		t.Fatalf("expected /query 200, got %d: %s", queryRec.Code, queryRec.Body.String())
	}

	var qResp QueryResponse
	if err := json.Unmarshal(queryRec.Body.Bytes(), &qResp); err != nil {
		t.Fatalf("failed to parse query json response: %v", err)
	}
	if !qResp.Success {
		t.Fatalf("expected success true in json response")
	}

	// 3. Rate limit rejection
	// Consume remaining tokens
	_ = limiter.Allow("corp_a")
	_ = limiter.Allow("corp_a")

	rateReq := httptest.NewRequest(http.MethodPost, "/query", bytes.NewBufferString(payload))
	rateRec := httptest.NewRecorder()
	proxy.HandleQuery(rateRec, rateReq)

	if rateRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rateRec.Code)
	}

	// 4. Metrics endpoint
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	proxy.HandleMetrics(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected /metrics 200, got %d", metricsRec.Code)
	}
	if !strings.Contains(metricsRec.Body.String(), "db_proxy_queries_total") {
		t.Fatalf("expected prometheus metrics format, got: %s", metricsRec.Body.String())
	}
}
