// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package mmr

import (
	"crypto/sha256"
	"errors"

	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

type Hash = types.Hash

// MerkleMountainRange represents an append-only cryptographic data structure
type MerkleMountainRange struct {
	peaks       []Hash
	peakHeights []int
	count       uint64
	leaves      []Hash
}

func NewMMR() *MerkleMountainRange {
	return &MerkleMountainRange{
		peaks:       make([]Hash, 0),
		peakHeights: make([]int, 0),
		count:       0,
		leaves:      make([]Hash, 0),
	}
}

func hashNode(left, right Hash) Hash {
	h := sha256.New()
	h.Write(left[:])
	h.Write(right[:])
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// Append inserts a new leaf into the MMR and performs binary mountain merges
func (m *MerkleMountainRange) Append(leaf Hash) uint64 {
	m.leaves = append(m.leaves, leaf)
	leafIdx := m.count
	m.count++

	currHash := leaf
	currHeight := 0

	// Binary carry addition: merge with preceding mountain while heights match
	for len(m.peakHeights) > 0 && m.peakHeights[len(m.peakHeights)-1] == currHeight {
		leftHash := m.peaks[len(m.peaks)-1]
		m.peaks = m.peaks[:len(m.peaks)-1]
		m.peakHeights = m.peakHeights[:len(m.peakHeights)-1]

		currHash = hashNode(leftHash, currHash)
		currHeight++
	}

	m.peaks = append(m.peaks, currHash)
	m.peakHeights = append(m.peakHeights, currHeight)

	return leafIdx
}

// GetPeaks returns a copy of all current mountain peak hashes
func (m *MerkleMountainRange) GetPeaks() []Hash {
	cp := make([]Hash, len(m.peaks))
	copy(cp, m.peaks)
	return cp
}

// BagPeaks combines all mountain peaks into a single root hash
func (m *MerkleMountainRange) BagPeaks() Hash {
	if len(m.peaks) == 0 {
		return Hash{}
	}
	if len(m.peaks) == 1 {
		return m.peaks[0]
	}

	bag := m.peaks[len(m.peaks)-1]
	for i := len(m.peaks) - 2; i >= 0; i-- {
		bag = hashNode(m.peaks[i], bag)
	}
	return bag
}

// Proof contains authentication siblings and peak context for a leaf
type Proof struct {
	LeafIndex uint64
	TotalSize uint64
	Siblings  []Hash
	Peaks     []Hash
}

// GenerateProof produces an inclusion proof for a leaf
func (m *MerkleMountainRange) GenerateProof(leafIndex uint64) (*Proof, error) {
	if leafIndex >= m.count {
		return nil, errors.New("leaf index out of range")
	}

	// 1. Locate which mountain contains the leaf
	var offset uint64 = 0
	targetMountainIdx := -1
	targetHeight := 0

	for i, h := range m.peakHeights {
		mountainLeaves := uint64(1) << h
		if leafIndex < offset+mountainLeaves {
			targetMountainIdx = i
			targetHeight = h
			break
		}
		offset += mountainLeaves
	}

	if targetMountainIdx < 0 {
		return nil, errors.New("mountain not found")
	}

	// 2. Build mountain tree levels to extract path siblings
	mountainLeavesCount := uint64(1) << targetHeight
	treeLevels := make([][]Hash, targetHeight+1)
	treeLevels[0] = make([]Hash, mountainLeavesCount)
	copy(treeLevels[0], m.leaves[offset:offset+mountainLeavesCount])

	for l := 0; l < targetHeight; l++ {
		nextLen := len(treeLevels[l]) / 2
		treeLevels[l+1] = make([]Hash, nextLen)
		for j := 0; j < nextLen; j++ {
			treeLevels[l+1][j] = hashNode(treeLevels[l][2*j], treeLevels[l][2*j+1])
		}
	}

	// 3. Collect siblings along the path
	siblings := make([]Hash, targetHeight)
	currIdx := leafIndex - offset
	for l := 0; l < targetHeight; l++ {
		isRight := (currIdx & 1) == 1
		if isRight {
			siblings[l] = treeLevels[l][currIdx-1]
		} else {
			siblings[l] = treeLevels[l][currIdx+1]
		}
		currIdx >>= 1
	}

	return &Proof{
		LeafIndex: leafIndex,
		TotalSize: m.count,
		Siblings:  siblings,
		Peaks:     m.GetPeaks(),
	}, nil
}

// VerifyProof verifies a leaf inclusion proof against the bagged peaks root
func VerifyProof(leaf Hash, proof *Proof, expectedRoot Hash) bool {
	if len(proof.Peaks) == 0 {
		return false
	}

	// 1. Reconstruct mountain peak from leaf and siblings
	// We need to determine the direction of each sibling from the leaf index
	curr := leaf
	var offset uint64 = 0
	for _, p := range proof.Peaks {
		_ = p
	}

	// Sibling climb
	currIdx := proof.LeafIndex
	for _, sib := range proof.Siblings {
		isRight := (currIdx & 1) == 1
		if isRight {
			curr = hashNode(sib, curr)
		} else {
			curr = hashNode(curr, sib)
		}
		currIdx >>= 1
	}

	// Verify that curr matches one of the peaks in proof.Peaks
	matchedPeak := false
	for _, peak := range proof.Peaks {
		if peak == curr {
			matchedPeak = true
			break
		}
	}
	if !matchedPeak {
		return false
	}

	// 2. Bag peaks and check root
	bag := proof.Peaks[len(proof.Peaks)-1]
	for i := len(proof.Peaks) - 2; i >= 0; i-- {
		bag = hashNode(proof.Peaks[i], bag)
	}

	return bag == expectedRoot && offset == 0
}
