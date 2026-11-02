// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package dispute

import (
	"errors"

	"github.com/ZengLLQ/zenith-sequencer/pkg/sequencer"
	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

type Hash = types.Hash
type ExecutionTraceStep = sequencer.ExecutionTraceStep

type DisputeStatus string

const (
	DisputeInProgress        DisputeStatus = "IN_PROGRESS"
	DisputeProposerSlashed   DisputeStatus = "PROPOSER_SLASHED"
	DisputeChallengerSlashed DisputeStatus = "CHALLENGER_SLASHED"
)

// DisputeGame manages the interactive bisection protocol between Proposer and Challenger
type DisputeGame struct {
	BatchID         uint64
	ProposerTrace   []ExecutionTraceStep
	ChallengerTrace []ExecutionTraceStep
	LowStep         uint64
	HighStep        uint64
	RoundsCompleted uint64
	Status          DisputeStatus
	DisputedStep    *ExecutionTraceStep
}

func NewDisputeGame(batchID uint64, proposerTrace, challengerTrace []ExecutionTraceStep) (*DisputeGame, error) {
	if len(proposerTrace) == 0 || len(challengerTrace) == 0 {
		return nil, errors.New("traces cannot be empty")
	}
	if len(proposerTrace) != len(challengerTrace) {
		return nil, errors.New("trace length mismatch")
	}

	n := uint64(len(proposerTrace))
	// Verify that pre-states agree at step 0
	if proposerTrace[0].PreState != challengerTrace[0].PreState {
		return nil, errors.New("initial pre-states must agree")
	}
	// Verify that post-states disagree at step n-1
	if proposerTrace[n-1].PostState == challengerTrace[n-1].PostState {
		return nil, errors.New("final post-states must disagree to initiate dispute")
	}

	return &DisputeGame{
		BatchID:         batchID,
		ProposerTrace:   proposerTrace,
		ChallengerTrace: challengerTrace,
		LowStep:         0,
		HighStep:        n - 1,
		RoundsCompleted: 0,
		Status:          DisputeInProgress,
	}, nil
}

// BisectStep performs one bisection round narrowing the disagreement window
func (g *DisputeGame) BisectStep() (bool, error) {
	if g.Status != DisputeInProgress {
		return true, errors.New("dispute game already finalized")
	}

	if g.LowStep == g.HighStep {
		// Disagreement narrowed to exactly 1 single transaction step
		step := g.ProposerTrace[g.LowStep]
		g.DisputedStep = &step
		return true, nil
	}

	mid := (g.LowStep + g.HighStep) / 2
	g.RoundsCompleted++

	// Check whether parties agree at mid step post-state
	if g.ProposerTrace[mid].PostState == g.ChallengerTrace[mid].PostState {
		// Agree up to mid -> disagreement is in upper half (mid+1 to high)
		g.LowStep = mid + 1
	} else {
		// Disagree at mid -> disagreement is in lower half (low to mid)
		g.HighStep = mid
	}

	if g.LowStep == g.HighStep {
		step := g.ProposerTrace[g.LowStep]
		g.DisputedStep = &step
		return true, nil
	}

	return false, nil
}

// ResolveOneStepProof verifies the single disputed transaction against canonical execution
func (g *DisputeGame) ResolveOneStepProof(seq *sequencer.Sequencer, tx *types.Transaction) (DisputeStatus, error) {
	if g.DisputedStep == nil {
		return DisputeInProgress, errors.New("bisection not yet converged to single step")
	}

	// Canonical verification: Execute single step starting from agreed PreState
	preState := g.DisputedStep.PreState

	// Verify pre-state matches current engine state fingerprint
	if seq.StateFingerprint() != preState {
		return DisputeInProgress, errors.New("engine state does not match agreed pre-state")
	}

	// Execute transaction canonically
	err := seq.ExecuteSingleTx(tx)
	if err != nil {
		// Execution failure
		return DisputeInProgress, err
	}

	actualPostState := seq.StateFingerprint()

	// Compare with Proposer's claim
	if actualPostState == g.DisputedStep.PostState {
		// Proposer was honest! Challenger submitted invalid challenge -> Slash challenger
		g.Status = DisputeChallengerSlashed
	} else {
		// Proposer committed fraud! PostState does not match canonical -> Slash proposer
		g.Status = DisputeProposerSlashed
	}

	return g.Status, nil
}
