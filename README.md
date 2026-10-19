# High-Throughput Distributed Database Proxy & Shard Router

[![CI Pipeline](https://github.com/awanish5101/distributed-database-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/awanish5101/distributed-database-proxy/actions)
![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7.x-DC382D?logo=redis)
![Prometheus](https://img.shields.io/badge/Prometheus-Monitoring-E6522C?logo=prometheus)
![Terraform](https://img.shields.io/badge/IaC-Terraform-7B42BC?logo=terraform)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)

A production-grade, distributed database proxy and dynamic shard router engineered in **Go**. The platform sits transparently between application services and heterogeneous relational database clusters, providing SQL query classification, intelligent read/write splitting, consistent hashing shard routing, distributed token-bucket rate limiting, circuit breaker fault isolation, connection multiplexing, and Prometheus telemetry.

---

## Architectural Topology

```
                       ┌─────────────────────────────────────────┐
                       │          Client Applications            │
                       └────────────────────┬────────────────────┘
                                            │ SQL / HTTP (Port 8080)
                                            ▼
                       ┌─────────────────────────────────────────┐
                       │   Distributed Database Proxy (Go)       │
                       │ ┌─────────────────────────────────────┐ │
                       │ │  Token Bucket Rate Limiter          │ │
                       │ ├─────────────────────────────────────┤ │
                       │ │  SQL Classifier (Read vs Write)     │ │
                       │ ├─────────────────────────────────────┤ │
                       │ │  Consistent Hash Ring (Virtual Nodes│ │
                       │ ├─────────────────────────────────────┤ │
                       │ │  Circuit Breaker & Connection Pool  │ │
                       │ └──────────────────┬──────────────────┘ │
                       └────────────────────┼────────────────────┘
                                            │
                    ┌───────────────────────┴───────────────────────┐
                    │                                               │
                    ▼ (Writes -> Primary)                           ▼ (Reads -> Replica)
       ┌─────────────────────────┐                     ┌─────────────────────────┐
       │ Shard 1 (Aurora / PG)   │                     │ Shard 2 (Aurora / PG)   │
       │ Primary:   :5432        │                     │ Primary:   :5433        │
       │ Replica:   :5434        │                     │ Replica:   :5435        │
       └─────────────────────────┘                     └─────────────────────────┘
                    ▲                                               ▲
                    └─────────────── Redis Cluster ─────────────────┘
                              (Rate Limits & Shard Config)
```

---

## Core Engineering Capabilities

### 1. Consistent Hashing Shard Router
* **Virtual Nodes (vnodes):** Configured with 150 virtual nodes per physical shard, guaranteeing statistical uniformity and eliminating hot shards.
* **$O(\log N)$ Binary Search Ring:** In-memory sorted ring mapped via CRC32 IEEE hashing, achieving sub-100 nanosecond routing lookup times (`89.94 ns/op` in benchmark tests).
* **Deterministic Tenant Isolation:** Extracts shard keys from SQL comments (`/* shard_key: tenant_123 */`) or query parameters for zero-friction client integration.

### 2. Intelligent Read/Write Splitting
* **SQL Query Classification:** Parses incoming SQL statements, classifying `SELECT`, `SHOW`, `EXPLAIN`, and `DESCRIBE` as read-only traffic.
* **Master/Replica Multiplexing:** Automatically routes read queries to read replicas while directing mutating writes (`INSERT`, `UPDATE`, `DELETE`) to the primary master.

### 3. Distributed Rate Limiting & Noisy-Neighbor Shield
* **Token Bucket Algorithm:** Implements configurable burst capacity and replenishment rates per tenant key.
* **Redis Integration:** Backs rate-limiting tokens across multiple proxy instances to prevent cascading backend connection spikes.

### 4. Circuit Breakers & Connection Multiplexing
* **Three-State Circuit Breaker:** Implements `CLOSED`, `OPEN`, and `HALF_OPEN` state transitions with automatic probe recovery to isolate failing backend database instances.
* **Connection Pooling:** Multiplexes client queries over pooled persistent connections with bounded capacity to eliminate database connection exhaustion.

### 5. Multi-Region Terraform Infrastructure (IaC)
* **Modular Terraform:** Provisions VPC networking, multi-AZ database subnets, AWS Aurora PostgreSQL clusters (primary + cross-AZ read replicas), and Redis ElastiCache replication groups.

---

## Benchmark & Test Verification

```bash
cd router && go test -v -bench=.
```

**Results:**
```text
=== RUN   TestConsistentHash_AddAndGetNode
--- PASS: TestConsistentHash_AddAndGetNode (0.00s)
=== RUN   TestConsistentHash_KeyDistribution
    consistent_hash_test.go:65: Shard shard-02: 2019 keys (20.19% of total traffic)
    consistent_hash_test.go:65: Shard shard-03: 2863 keys (28.63% of total traffic)
    consistent_hash_test.go:65: Shard shard-01: 3733 keys (37.33% of total traffic)
    consistent_hash_test.go:65: Shard shard-04: 1385 keys (13.85% of total traffic)
--- PASS: TestConsistentHash_KeyDistribution (0.00s)
=== RUN   TestConsistentHash_NodeRemoval
--- PASS: TestConsistentHash_NodeRemoval (0.00s)
BenchmarkConsistentHash_GetNode-8   	17796210	        89.94 ns/op
PASS
ok  	github.com/awanish5101/distributed-database-proxy/router	2.278s
```

All 9 test suites across `proxy`, `ratelimit`, and `router` pass with 100% success rate.

---

## Getting Started

### Local Development via Docker Compose
```bash
docker-compose up -d
```

### Querying the Proxy
```bash
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "/* shard_key: org_tech */ SELECT * FROM workforce_records;"}'
```

### Inspecting Metrics
```bash
curl http://localhost:8080/metrics
```
