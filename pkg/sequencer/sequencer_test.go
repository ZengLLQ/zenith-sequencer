// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package sequencer

import (
	"math/big"
	"testing"

	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

func TestSequencer_TransactionExecutionAndBalanceAccounting(t *testing.T) {
	seq := NewSequencer()
	alice := Address{0x01}
	bob := Address{0x02}

	initialBal := big.NewInt(5_000_000_000_000_000_000) // 5 ETH
	seq.SetBalance(alice, initialBal)

	transferVal := big.NewInt(1_000_000_000_000_000_000) // 1 ETH
	gasPrice := big.NewInt(20_000_000_000)               // 20 Gwei
	gasLimit := uint64(21000)
	gasCost := new(big.Int).Mul(gasPrice, big.NewInt(int64(gasLimit)))

	tx := types.NewTransaction(alice, bob, 0, transferVal, nil, gasLimit, gasPrice)
	err := seq.AddTransaction(tx)
	if err != nil {
		t.Fatalf("failed to add transaction: %v", err)
	}

	batch, err := seq.ProduceBatch()
	if err != nil {
		t.Fatalf("failed to produce batch: %v", err)
	}

	if len(batch.Transactions) != 1 {
		t.Fatalf("expected 1 executed transaction, got %d", len(batch.Transactions))
	}

	expectedAlice := new(big.Int).Sub(initialBal, new(big.Int).Add(transferVal, gasCost))
	actualAlice := seq.GetBalance(alice)
	if actualAlice.Cmp(expectedAlice) != 0 {
		t.Fatalf("alice balance mismatch: expected %s, got %s", expectedAlice, actualAlice)
	}

	actualBob := seq.GetBalance(bob)
	if actualBob.Cmp(transferVal) != 0 {
		t.Fatalf("bob balance mismatch: expected %s, got %s", transferVal, actualBob)
	}
}

func TestSequencer_AtomicMEVBundleReversion(t *testing.T) {
	seq := NewSequencer()
	searcher := Address{0xAA}
	victim := Address{0xBB}

	seq.SetBalance(searcher, big.NewInt(1_000_000_000))
	seq.SetBalance(victim, big.NewInt(500)) // Victim only has 500

	// Bundle Tx 1: valid transfer of 100
	tx1 := types.NewTransaction(searcher, victim, 0, big.NewInt(100), nil, 21000, big.NewInt(1))
	// Bundle Tx 2: invalid transfer of 1000 from victim (insufficient funds!)
	tx2 := types.NewTransaction(victim, searcher, 0, big.NewInt(1000), nil, 21000, big.NewInt(1))

	bundle := &Bundle{
		Transactions: []*Transaction{tx1, tx2},
		Bribe:        big.NewInt(50),
	}
	seq.AddBundle(bundle)

	batch, err := seq.ProduceBatch()
	if err != nil {
		t.Fatalf("batch production failed: %v", err)
	}

	// Because tx2 failed, the entire bundle MUST revert atomically!
	if len(batch.Transactions) != 0 {
		t.Fatalf("expected 0 transactions executed due to bundle revert, got %d", len(batch.Transactions))
	}

	// Balances must remain strictly unchanged
	if seq.GetBalance(searcher).Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("searcher balance changed despite bundle revert")
	}
	if seq.GetBalance(victim).Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("victim balance changed despite bundle revert")
	}
}
