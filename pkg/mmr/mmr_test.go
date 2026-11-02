// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package mmr

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func makeTestLeaf(val uint64) Hash {
	h := sha256.New()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], val)
	h.Write(b[:])
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

func TestMMR_AppendAndPeakDecomposition(t *testing.T) {
	m := NewMMR()

	// Append 11 elements (11 = 8 + 2 + 1, so 3 mountains of sizes 8, 2, 1)
	for i := uint64(0); i < 11; i++ {
		m.Append(makeTestLeaf(i))
	}

	peaks := m.GetPeaks()
	if len(peaks) != 3 {
		t.Fatalf("expected 3 peaks for 11 leaves, got %d", len(peaks))
	}

	root := m.BagPeaks()
	if root == (Hash{}) {
		t.Fatalf("bagged peaks root must not be empty")
	}

	// Appending an element changes the root
	m.Append(makeTestLeaf(11))
	newRoot := m.BagPeaks()
	if root == newRoot {
		t.Fatalf("new append must change the bagged root")
	}
}

func TestMMR_InclusionProofs(t *testing.T) {
	m := NewMMR()
	leaves := make([]Hash, 15)
	for i := uint64(0); i < 15; i++ {
		leaves[i] = makeTestLeaf(i * 100)
		m.Append(leaves[i])
	}

	root := m.BagPeaks()

	// Verify proof generation and verification for all 15 leaves
	for i := uint64(0); i < 15; i++ {
		proof, err := m.GenerateProof(i)
		if err != nil {
			t.Fatalf("failed to generate proof for leaf %d: %v", i, err)
		}

		if proof.LeafIndex != i {
			t.Fatalf("proof leaf index mismatch: %d != %d", proof.LeafIndex, i)
		}

		valid := VerifyProof(leaves[i], proof, root)
		if !valid {
			t.Fatalf("proof verification failed for leaf %d", i)
		}

		// Tampering test: wrong leaf should fail
		tamperedLeaf := makeTestLeaf(99999)
		if VerifyProof(tamperedLeaf, proof, root) {
			t.Fatalf("tampered leaf must fail verification")
		}
	}
}
