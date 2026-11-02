// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package aequitas

import (
	"testing"
)

func makeHash(b byte) Hash {
	var h Hash
	h[0] = b
	return h
}

func TestAequitas_UnanimousOrdering(t *testing.T) {
	txA := makeHash(0xAA)
	txB := makeHash(0xBB)
	txC := makeHash(0xCC)

	reports := []NodeReport{
		{NodeID: "node1", ArrivalOrder: []Hash{txA, txB, txC}},
		{NodeID: "node2", ArrivalOrder: []Hash{txA, txB, txC}},
		{NodeID: "node3", ArrivalOrder: []Hash{txA, txB, txC}},
	}

	engine := NewFairOrderEngine(0.67)
	order := engine.ComputeFairOrder(reports)

	if len(order) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(order))
	}
	if order[0] != txA || order[1] != txB || order[2] != txC {
		t.Fatalf("unanimous order mismatch: got %v", order)
	}
}

func TestAequitas_CycleResolutionViaSCC(t *testing.T) {
	// Adversarial cycle: A -> B -> C -> A
	txA := makeHash(0x01)
	txB := makeHash(0x02)
	txC := makeHash(0x03)

	reports := []NodeReport{
		{NodeID: "node1", ArrivalOrder: []Hash{txA, txB, txC}}, // A before B, B before C
		{NodeID: "node2", ArrivalOrder: []Hash{txB, txC, txA}}, // B before C, C before A
		{NodeID: "node3", ArrivalOrder: []Hash{txC, txA, txB}}, // C before A, A before B
	}

	engine := NewFairOrderEngine(0.60)
	order := engine.ComputeFairOrder(reports)

	if len(order) != 3 {
		t.Fatalf("expected 3 transactions in cycle, got %d", len(order))
	}

	// All 3 elements must be present
	seen := make(map[Hash]bool)
	for _, h := range order {
		seen[h] = true
	}
	if !seen[txA] || !seen[txB] || !seen[txC] {
		t.Fatalf("missing transaction from cycle resolution")
	}
}

func TestAequitas_MajorityPrecedence(t *testing.T) {
	txA := makeHash(0x10)
	txB := makeHash(0x20)

	// 3 out of 4 nodes (75% > 67%) see txA before txB
	reports := []NodeReport{
		{NodeID: "node1", ArrivalOrder: []Hash{txA, txB}},
		{NodeID: "node2", ArrivalOrder: []Hash{txA, txB}},
		{NodeID: "node3", ArrivalOrder: []Hash{txA, txB}},
		{NodeID: "node4", ArrivalOrder: []Hash{txB, txA}}, // Malicious / lagging node
	}

	engine := NewFairOrderEngine(0.67)
	order := engine.ComputeFairOrder(reports)

	if len(order) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(order))
	}
	if order[0] != txA || order[1] != txB {
		t.Fatalf("expected txA before txB due to 75%% majority precedence, got %v", order)
	}
}
