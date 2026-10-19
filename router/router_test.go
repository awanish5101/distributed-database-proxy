package router

import (
	"testing"
)

func TestRouter_ClassifyQuery(t *testing.T) {
	r := NewRouter(50)

	readQueries := []string{
		"SELECT * FROM users WHERE id = 1;",
		"   select count(*) from orders",
		"SHOW TABLES;",
		"EXPLAIN ANALYZE SELECT * FROM audit_logs;",
		"DESCRIBE accounts;",
	}

	for _, q := range readQueries {
		if r.ClassifyQuery(q) != QueryTypeRead {
			t.Errorf("expected READ for query: %s", q)
		}
	}

	writeQueries := []string{
		"INSERT INTO users (name) VALUES ('Alice');",
		"UPDATE accounts SET balance = balance + 100 WHERE id = 5;",
		"DELETE FROM sessions WHERE expired_at < NOW();",
		"CREATE TABLE items (id serial primary key);",
		"DROP TABLE temp_data;",
	}

	for _, q := range writeQueries {
		if r.ClassifyQuery(q) != QueryTypeWrite {
			t.Errorf("expected WRITE for query: %s", q)
		}
	}
}

func TestRouter_ExtractShardKey(t *testing.T) {
	r := NewRouter(50)

	sql1 := "/* shard_key: org_987 */ SELECT * FROM invoices;"
	if key := r.ExtractShardKey(sql1, ""); key != "org_987" {
		t.Errorf("expected org_987, got %s", key)
	}

	sql2 := "/* tenant_id=customer_44 */ INSERT INTO logs VALUES (1);"
	if key := r.ExtractShardKey(sql2, ""); key != "customer_44" {
		t.Errorf("expected customer_44, got %s", key)
	}

	sql3 := "SELECT * FROM global_config;"
	if key := r.ExtractShardKey(sql3, "fallback_corp"); key != "fallback_corp" {
		t.Errorf("expected fallback_corp, got %s", key)
	}
}

func TestRouter_RouteReadWriteSplitting(t *testing.T) {
	r := NewRouter(50)

	r.RegisterShard(&ShardInfo{
		ID:       "shard-us-east-1",
		Primary:  "postgres-primary-1:5432",
		Replicas: []string{"postgres-replica-1:5432"},
		IsAlive:  true,
	})

	// Read query should route to replica
	readSQL := "/* shard_key: tenant_10 */ SELECT * FROM payroll WHERE id = 1;"
	decRead, err := r.Route(readSQL, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decRead.IsReplica || decRead.TargetAddress != "postgres-replica-1:5432" {
		t.Errorf("read query not routed to replica: %+v", decRead)
	}

	// Write query should route to primary
	writeSQL := "/* shard_key: tenant_10 */ UPDATE payroll SET salary = 90000 WHERE id = 1;"
	decWrite, err := r.Route(writeSQL, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decWrite.IsReplica || decWrite.TargetAddress != "postgres-primary-1:5432" {
		t.Errorf("write query not routed to primary: %+v", decWrite)
	}
}
