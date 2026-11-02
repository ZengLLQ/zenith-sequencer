// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package dispute

import (
	"math/big"
	"testing"

	"github.com/ZengLLQ/zenith-sequencer/pkg/sequencer"
	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

func TestDispute_BisectionConvergence(t *testing.T) {
	seq := sequencer.NewSequencer()
	alice := types.Address{0x01}
	bob := types.Address{0x02}

	seq.SetBalance(alice, big.NewInt(100_000_000_000))

	// Queue 8 transactions to form a batch of 8 steps
	for i := uint64(0); i < 8; i++ {
		tx := types.NewTransaction(alice, bob, i, big.NewInt(100), nil, 21000, big.NewInt(1))
		_ = seq.AddTransaction(tx)
	}

	batch, err := seq.ProduceBatch()
	if err != nil {
		t.Fatalf("failed to produce batch: %v", err)
	}
	if len(batch.Trace) != 8 {
		t.Fatalf("expected trace of 8 steps, got %d", len(batch.Trace))
	}

	honestTrace := make([]sequencer.ExecutionTraceStep, 8)
	copy(honestTrace, batch.Trace)

	fraudulentTrace := make([]sequencer.ExecutionTraceStep, 8)
	copy(fraudulentTrace, batch.Trace)

	// Inject fraudulent state claim at step 5
	disputedIndex := uint64(5)
	fraudulentTrace[disputedIndex].PostState = types.Hash{0xde, 0xad, 0xbe, 0xef}
	for i := disputedIndex + 1; i < 8; i++ {
		fraudulentTrace[i].PreState = fraudulentTrace[i-1].PostState
		fraudulentTrace[i].PostState = types.Hash{byte(i), 0xfe, 0xed}
	}

	game, err := NewDisputeGame(batch.BatchNumber, fraudulentTrace, honestTrace)
	if err != nil {
		t.Fatalf("failed to initialize dispute: %v", err)
	}

	// Interactive bisection rounds
	for {
		converged, err := game.BisectStep()
		if err != nil {
			t.Fatalf("bisection step failed: %v", err)
		}
		if converged {
			break
		}
	}

	if game.DisputedStep == nil {
		t.Fatalf("expected converged disputed step")
	}

	if game.LowStep != disputedIndex {
		t.Fatalf("expected bisection to converge to disputed step %d, got %d", disputedIndex, game.LowStep)
	}

	// For 8 steps, bisection takes exactly log2(8) = 3 rounds
	if game.RoundsCompleted != 3 {
		t.Fatalf("expected 3 bisection rounds for 8 steps, got %d", game.RoundsCompleted)
	}
}
