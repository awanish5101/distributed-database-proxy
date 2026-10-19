package router

import (
	"fmt"
	"testing"
)

func TestConsistentHash_AddAndGetNode(t *testing.T) {
	ring := NewHashRing(100)

	// Ring empty check
	_, err := ring.GetNode("tenant_123")
	if err == nil {
		t.Fatalf("expected error on empty ring, got nil")
	}

	// Add nodes
	ring.AddNode("shard-01")
	ring.AddNode("shard-02")
	ring.AddNode("shard-03")

	if ring.NodeCount() != 3 {
		t.Fatalf("expected 3 nodes, got %d", ring.NodeCount())
	}

	// Route keys deterministically
	node1, err := ring.GetNode("tenant_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node2, err := ring.GetNode("tenant_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node1 != node2 {
		t.Fatalf("inconsistent routing for same key: %s != %s", node1, node2)
	}
}

func TestConsistentHash_KeyDistribution(t *testing.T) {
	vnodes := 100
	ring := NewHashRing(vnodes)
	shards := []string{"shard-01", "shard-02", "shard-03", "shard-04"}

	for _, s := range shards {
		ring.AddNode(s)
	}

	keyCount := 10000
	distribution := make(map[string]int)

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("tenant_key_%d", i)
		node, err := ring.GetNode(key)
		if err != nil {
			t.Fatalf("routing error: %v", err)
		}
		distribution[node]++
	}

	for shard, count := range distribution {
		sharePct := (float64(count) / float64(keyCount)) * 100
		t.Logf("Shard %s: %d keys (%.2f%% of total traffic)", shard, count, sharePct)
		// Ensure no node is starved (<10%) or monopolized (>45%)
		if sharePct < 10.0 || sharePct > 45.0 {
			t.Errorf("Shard %s traffic share out of expected balanced range (10%%-45%%): %.2f%%", shard, sharePct)
		}
	}
}

func TestConsistentHash_NodeRemoval(t *testing.T) {
	ring := NewHashRing(100)
	ring.AddNode("shard-01")
	ring.AddNode("shard-02")

	key := "persistent_tenant_999"
	firstNode, _ := ring.GetNode(key)

	// Remove the other node or the selected node
	ring.RemoveNode("shard-01")

	secondNode, err := ring.GetNode(key)
	if err != nil {
		t.Fatalf("unexpected error after removal: %v", err)
	}

	if secondNode != "shard-02" {
		t.Fatalf("expected shard-02 after shard-01 removal, got %s", secondNode)
	}
	_ = firstNode
}

func BenchmarkConsistentHash_GetNode(b *testing.B) {
	ring := NewHashRing(100)
	for i := 1; i <= 10; i++ {
		ring.AddNode(fmt.Sprintf("shard-%02d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ring.GetNode("benchmark_tenant_key")
	}
}
