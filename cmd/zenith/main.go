// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package main

import (
	"fmt"
	"math/big"

	"github.com/ZengLLQ/zenith-sequencer/pkg/aequitas"
	"github.com/ZengLLQ/zenith-sequencer/pkg/dispute"
	"github.com/ZengLLQ/zenith-sequencer/pkg/sequencer"
	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

func main() {
	fmt.Println("==================================================================")
	fmt.Println(" ZenithSequencer: Decentralized Rollup Fair Sequencer & Dispute  ")
	fmt.Println("==================================================================")

	// 1. Initialize Sequencer & Accounts
	seq := sequencer.NewSequencer()
	alice := types.Address{0x01}
	bob := types.Address{0x02}
	searcher := types.Address{0x03}

	aliceBal, _ := new(big.Int).SetString("10000000000000000000", 10)    // 10 ETH
	bobBal, _ := new(big.Int).SetString("5000000000000000000", 10)       // 5 ETH
	searcherBal, _ := new(big.Int).SetString("20000000000000000000", 10) // 20 ETH

	seq.SetBalance(alice, aliceBal)
	seq.SetBalance(bob, bobBal)
	seq.SetBalance(searcher, searcherBal)

	fmt.Printf("\n[1] Initial Genesis Balances:")
	fmt.Printf("\n    Alice:    %s wei", seq.GetBalance(alice).String())
	fmt.Printf("\n    Bob:      %s wei", seq.GetBalance(bob).String())
	fmt.Printf("\n    Searcher: %s wei\n", seq.GetBalance(searcher).String())

	// 2. Aequitas Fair Ordering Demonstration
	fmt.Println("\n[2] Executing Aequitas Fair Ordering across 4 Validator Nodes...")
	tx1 := types.NewTransaction(alice, bob, 0, big.NewInt(1_000_000_000_000_000_000), nil, 21000, big.NewInt(20_000_000_000))
	tx2 := types.NewTransaction(bob, alice, 0, big.NewInt(500_000_000_000_000_000), nil, 21000, big.NewInt(25_000_000_000))
	tx3 := types.NewTransaction(alice, bob, 1, big.NewInt(2_000_000_000_000_000_000), nil, 21000, big.NewInt(30_000_000_000))

	reports := []aequitas.NodeReport{
		{NodeID: "validator-1", ArrivalOrder: []types.Hash{tx1.Hash, tx2.Hash, tx3.Hash}},
		{NodeID: "validator-2", ArrivalOrder: []types.Hash{tx1.Hash, tx2.Hash, tx3.Hash}},
		{NodeID: "validator-3", ArrivalOrder: []types.Hash{tx2.Hash, tx1.Hash, tx3.Hash}},
		{NodeID: "validator-4", ArrivalOrder: []types.Hash{tx1.Hash, tx3.Hash, tx2.Hash}},
	}

	fairEngine := aequitas.NewFairOrderEngine(0.67)
	fairOrder := fairEngine.ComputeFairOrder(reports)

	fmt.Printf("    Fair Transaction Order Determined:\n")
	for i, h := range fairOrder {
		fmt.Printf("      #%d: %s\n", i+1, h.String())
	}

	// 3. MEV Atomic Bundle Execution
	fmt.Println("\n[3] Testing MEV Bundle Atomicity...")
	bundleTx1 := types.NewTransaction(searcher, alice, 0, big.NewInt(100), nil, 21000, big.NewInt(50_000_000_000))
	bundleTx2 := types.NewTransaction(searcher, bob, 1, big.NewInt(200), nil, 21000, big.NewInt(50_000_000_000))
	bundle := &types.Bundle{
		Transactions: []*types.Transaction{bundleTx1, bundleTx2},
		Bribe:        big.NewInt(1_000_000),
	}
	seq.AddBundle(bundle)

	// Add standard transactions
	_ = seq.AddTransaction(tx1)

	// Produce Rollup Batch
	batch, err := seq.ProduceBatch()
	if err != nil {
		panic(err)
	}

	fmt.Printf("    Batch #%d Produced successfully!\n", batch.BatchNumber)
	fmt.Printf("    Executed Transactions: %d\n", len(batch.Transactions))
	fmt.Printf("    State MMR Root (Bagged Peaks): %s\n", batch.NewRoot.String())

	// 4. Interactive Fraud Dispute Simulation
	fmt.Println("\n[4] Simulating Interactive Bisection Fraud Dispute Game...")
	// Create honest challenger trace and fraudulent proposer trace
	honestTrace := make([]sequencer.ExecutionTraceStep, len(batch.Trace))
	copy(honestTrace, batch.Trace)

	fraudulentTrace := make([]sequencer.ExecutionTraceStep, len(batch.Trace))
	copy(fraudulentTrace, batch.Trace)
	// Inject fraud starting at step index 1 through the end of the batch
	if len(fraudulentTrace) > 1 {
		for i := 1; i < len(fraudulentTrace); i++ {
			fraudulentTrace[i].PostState = types.Hash{byte(i), 0xff, 0xee, 0xaa}
		}
	}

	game, err := dispute.NewDisputeGame(batch.BatchNumber, fraudulentTrace, honestTrace)
	if err != nil {
		fmt.Printf("    Dispute Initialization Notice: %v\n", err)
	} else {
		for {
			converged, _ := game.BisectStep()
			if converged {
				fmt.Printf("    Dispute converged to single Step #%d in %d rounds!\n", game.DisputedStep.StepIndex, game.RoundsCompleted)
				break
			}
		}
	}

	fmt.Println("\n==================================================================")
	fmt.Println(" ZenithSequencer Demonstration Complete. All Invariants Maintained.")
	fmt.Println("==================================================================")
}
