package router

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
)

// HashRing represents a thread-safe consistent hash ring with virtual nodes.
type HashRing struct {
	mu          sync.RWMutex
	vnodes      int               // Number of virtual nodes per physical node
	ring        []uint32          // Sorted array of hashed virtual node positions
	vnodeToNode map[uint32]string // Mapping from virtual node hash to physical node ID
	nodes       map[string]bool   // Set of active physical node IDs
}

// NewHashRing creates a new HashRing with the specified number of virtual nodes per physical node.
func NewHashRing(vnodes int) *HashRing {
	if vnodes <= 0 {
		vnodes = 150
	}
	return &HashRing{
		vnodes:      vnodes,
		ring:        make([]uint32, 0),
		vnodeToNode: make(map[uint32]string),
		nodes:       make(map[string]bool),
	}
}

// hashKey computes a 32-bit CRC32 IEEE checksum of the given key.
func hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// AddNode adds a physical node and its virtual nodes to the hash ring.
func (h *HashRing) AddNode(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nodes[node] {
		return
	}
	h.nodes[node] = true

	for i := 0; i < h.vnodes; i++ {
		vnodeKey := fmt.Sprintf("%s#vn%s", node, strconv.Itoa(i))
		hash := hashKey(vnodeKey)
		h.ring = append(h.ring, hash)
		h.vnodeToNode[hash] = node
	}

	sort.Slice(h.ring, func(i, j int) bool {
		return h.ring[i] < h.ring[j]
	})
}

// RemoveNode removes a physical node and its virtual nodes from the hash ring.
func (h *HashRing) RemoveNode(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.nodes[node] {
		return
	}
	delete(h.nodes, node)

	newRing := make([]uint32, 0, len(h.ring)-(h.vnodes))
	for _, hash := range h.ring {
		if h.vnodeToNode[hash] == node {
			delete(h.vnodeToNode, hash)
		} else {
			newRing = append(newRing, hash)
		}
	}
	h.ring = newRing
}

// GetNode routes a key to the closest physical node on the hash ring (clockwise).
func (h *HashRing) GetNode(key string) (string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 {
		return "", fmt.Errorf("hash ring is empty; no nodes available")
	}

	hash := hashKey(key)

	// Binary search for the first virtual node with hash >= key's hash
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i] >= hash
	})

	// Wrap around to index 0 if hash is greater than all nodes in the ring
	if idx == len(h.ring) {
		idx = 0
	}

	vnodeHash := h.ring[idx]
	return h.vnodeToNode[vnodeHash], nil
}

// NodeCount returns the number of active physical nodes.
func (h *HashRing) NodeCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}
