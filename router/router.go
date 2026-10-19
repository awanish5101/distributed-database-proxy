package router

import (
	"errors"
	"regexp"
	"strings"
	"sync"
)

// QueryType defines whether a query is Read-Only or Write.
type QueryType string

const (
	QueryTypeRead  QueryType = "READ"
	QueryTypeWrite QueryType = "WRITE"
)

// ShardInfo contains connection and topology details for a database shard.
type ShardInfo struct {
	ID       string `json:"id"`
	Primary  string `json:"primary"`  // Connection string or address for primary/master
	Replicas []string `json:"replicas"` // Connection strings or addresses for read replicas
	IsAlive  bool   `json:"is_alive"`
}

// Router orchestrates query classification, shard routing, and primary/replica selection.
type Router struct {
	mu           sync.RWMutex
	ring         *HashRing
	shards       map[string]*ShardInfo
	readPatterns []*regexp.Regexp
	shardKeyRx   *regexp.Regexp
}

// NewRouter instantiates a query router.
func NewRouter(vnodes int) *Router {
	return &Router{
		ring:   NewHashRing(vnodes),
		shards: make(map[string]*ShardInfo),
		readPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*(SELECT|SHOW|EXPLAIN|DESCRIBE)\b`),
		},
		shardKeyRx: regexp.MustCompile(`(?i)/\*+\s*(?:shard_key|tenant_id)\s*[:=]\s*([a-zA-Z0-9_\-]+)\s*\*+/`),
	}
}

// RegisterShard registers a shard cluster (primary + read replicas) to the router.
func (r *Router) RegisterShard(shard *ShardInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.shards[shard.ID] = shard
	r.ring.AddNode(shard.ID)
}

// UnregisterShard removes a shard cluster from the active routing ring.
func (r *Router) UnregisterShard(shardID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.shards, shardID)
	r.ring.RemoveNode(shardID)
}

var commentRx = regexp.MustCompile(`(?s)^\s*(?:/\*.*?\*/|--[^\n]*\n\s*)*`)

// ClassifyQuery determines whether a SQL statement is READ or WRITE.
func (r *Router) ClassifyQuery(sql string) QueryType {
	cleanSQL := commentRx.ReplaceAllString(sql, "")
	cleanSQL = strings.TrimSpace(cleanSQL)
	for _, p := range r.readPatterns {
		if p.MatchString(cleanSQL) {
			return QueryTypeRead
		}
	}
	return QueryTypeWrite
}

// ExtractShardKey extracts the tenant/shard key from SQL comments or returns a fallback key.
func (r *Router) ExtractShardKey(sql, fallbackKey string) string {
	matches := r.shardKeyRx.FindStringSubmatch(sql)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	if fallbackKey != "" {
		return fallbackKey
	}
	return "default"
}

// RouteDecision contains the target shard and target node address (primary or replica).
type RouteDecision struct {
	ShardID       string    `json:"shard_id"`
	TargetAddress string    `json:"target_address"`
	QueryType     QueryType `json:"query_type"`
	ShardKey      string    `json:"shard_key"`
	IsReplica     bool      `json:"is_replica"`
}

// Route determines the target shard and backend instance for the given SQL query.
func (r *Router) Route(sql, fallbackKey string) (*RouteDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.shards) == 0 {
		return nil, errors.New("no database shards available in router")
	}

	shardKey := r.ExtractShardKey(sql, fallbackKey)
	shardID, err := r.ring.GetNode(shardKey)
	if err != nil {
		return nil, err
	}

	shard, exists := r.shards[shardID]
	if !exists {
		return nil, errors.New("selected shard not registered in topology")
	}

	qType := r.ClassifyQuery(sql)
	decision := &RouteDecision{
		ShardID:   shardID,
		QueryType: qType,
		ShardKey:  shardKey,
	}

	if qType == QueryTypeRead && len(shard.Replicas) > 0 {
		// Route reads to read replica (simple round-robin or first replica)
		decision.TargetAddress = shard.Replicas[0]
		decision.IsReplica = true
	} else {
		// Route writes (or reads with no replica) to primary master
		decision.TargetAddress = shard.Primary
		decision.IsReplica = false
	}

	return decision, nil
}
