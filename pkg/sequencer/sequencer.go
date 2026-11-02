// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package sequencer

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ZengLLQ/zenith-sequencer/pkg/mmr"
	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

type Address = types.Address
type Hash = types.Hash
type Transaction = types.Transaction
type Bundle = types.Bundle
type AccountState = types.AccountState

// ExecutionTraceStep records a single state transition step for interactive dispute resolution
type ExecutionTraceStep struct {
	StepIndex uint64
	TxHash    Hash
	PreState  Hash
	PostState Hash
}

// RollupBatch represents a finalized sequence of transactions committed to L1
type RollupBatch struct {
	BatchNumber  uint64
	PrevRoot     Hash
	NewRoot      Hash
	Transactions []*Transaction
	Timestamp    uint64
	Trace        []ExecutionTraceStep
}

// Sequencer manages the L2 execution environment and batch production
type Sequencer struct {
	mu           sync.RWMutex
	accounts     map[Address]*AccountState
	stateMMR     *mmr.MerkleMountainRange
	batches      []*RollupBatch
	mempool      []*Transaction
	bundles      []*Bundle
	currentBatch uint64
}

func NewSequencer() *Sequencer {
	return &Sequencer{
		accounts:     make(map[Address]*AccountState),
		stateMMR:     mmr.NewMMR(),
		batches:      make([]*RollupBatch, 0),
		mempool:      make([]*Transaction, 0),
		bundles:      make([]*Bundle, 0),
		currentBatch: 1,
	}
}

// SetBalance sets an initial balance for an account (genesis/deposit)
func (s *Sequencer) SetBalance(addr Address, bal *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acct, ok := s.accounts[addr]
	if !ok {
		acct = types.NewAccountState()
		s.accounts[addr] = acct
	}
	acct.Balance = new(big.Int).Set(bal)
}

// GetBalance returns the balance of an account
func (s *Sequencer) GetBalance(addr Address) *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acct, ok := s.accounts[addr]
	if !ok {
		return big.NewInt(0)
	}
	return new(big.Int).Set(acct.Balance)
}

// AddTransaction submits a transaction to the mempool
func (s *Sequencer) AddTransaction(tx *Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acct, ok := s.accounts[tx.Sender]
	if !ok {
		return errors.New("sender account does not exist")
	}
	if tx.Nonce < acct.Nonce {
		return errors.New("nonce too low")
	}

	totalCost := new(big.Int).Add(tx.Value, new(big.Int).Mul(tx.GasPrice, big.NewInt(int64(tx.GasLimit))))
	if acct.Balance.Cmp(totalCost) < 0 {
		return errors.New("insufficient balance for transfer and gas")
	}

	s.mempool = append(s.mempool, tx)
	return nil
}

// AddBundle submits an atomic MEV bundle
func (s *Sequencer) AddBundle(bundle *Bundle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bundles = append(s.bundles, bundle)
}

// StateFingerprint computes a deterministic root hash of all accounts
func (s *Sequencer) StateFingerprint() Hash {
	h := sha256.New()
	for addr, acct := range s.accounts {
		h.Write(addr[:])
		var nBuf [8]byte
		binary.BigEndian.PutUint64(nBuf[:], acct.Nonce)
		h.Write(nBuf[:])
		h.Write(acct.Balance.Bytes())
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// ExecuteSingleTx executes a single transaction modifying accounts
func (s *Sequencer) ExecuteSingleTx(tx *Transaction) error {
	senderAcct, ok := s.accounts[tx.Sender]
	if !ok {
		return errors.New("sender does not exist")
	}

	gasFee := new(big.Int).Mul(tx.GasPrice, big.NewInt(int64(tx.GasLimit)))
	totalDebit := new(big.Int).Add(tx.Value, gasFee)

	if senderAcct.Balance.Cmp(totalDebit) < 0 {
		return errors.New("insufficient balance")
	}

	// Debit sender
	senderAcct.Balance.Sub(senderAcct.Balance, totalDebit)
	senderAcct.Nonce++

	// Credit recipient
	recipientAcct, ok := s.accounts[tx.To]
	if !ok {
		recipientAcct = types.NewAccountState()
		s.accounts[tx.To] = recipientAcct
	}
	recipientAcct.Balance.Add(recipientAcct.Balance, tx.Value)

	return nil
}

// ProduceBatch processes queued transactions and bundles, committing state to MMR
func (s *Sequencer) ProduceBatch() (*RollupBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prevRoot := s.stateMMR.BagPeaks()
	executedTxs := make([]*Transaction, 0)
	trace := make([]ExecutionTraceStep, 0)

	// 1. Execute bundles atomically with snapshot revert capability
	for _, bundle := range s.bundles {
		snapshot := make(map[Address]*AccountState)
		for k, v := range s.accounts {
			snapshot[k] = v.Clone()
		}

		allSucceeded := true
		bundleTxs := make([]*Transaction, 0)
		bundleTrace := make([]ExecutionTraceStep, 0)

		for _, tx := range bundle.Transactions {
			preHash := s.StateFingerprint()
			err := s.ExecuteSingleTx(tx)
			if err != nil {
				allSucceeded = false
				break
			}
			postHash := s.StateFingerprint()
			bundleTxs = append(bundleTxs, tx)
			bundleTrace = append(bundleTrace, ExecutionTraceStep{
				StepIndex: uint64(len(executedTxs) + len(bundleTxs)),
				TxHash:    tx.Hash,
				PreState:  preHash,
				PostState: postHash,
			})
		}

		if allSucceeded {
			executedTxs = append(executedTxs, bundleTxs...)
			trace = append(trace, bundleTrace...)
		} else {
			// Revert to snapshot
			s.accounts = snapshot
		}
	}
	s.bundles = make([]*Bundle, 0) // Drain bundles

	// 2. Execute standard mempool transactions
	for _, tx := range s.mempool {
		preHash := s.StateFingerprint()
		if err := s.ExecuteSingleTx(tx); err == nil {
			postHash := s.StateFingerprint()
			executedTxs = append(executedTxs, tx)
			trace = append(trace, ExecutionTraceStep{
				StepIndex: uint64(len(executedTxs)),
				TxHash:    tx.Hash,
				PreState:  preHash,
				PostState: postHash,
			})
		}
	}
	s.mempool = make([]*Transaction, 0) // Drain mempool

	// 3. Commit new state to MMR
	finalStateFingerprint := s.StateFingerprint()
	s.stateMMR.Append(finalStateFingerprint)
	newRoot := s.stateMMR.BagPeaks()

	batch := &RollupBatch{
		BatchNumber:  s.currentBatch,
		PrevRoot:     prevRoot,
		NewRoot:      newRoot,
		Transactions: executedTxs,
		Timestamp:    uint64(time.Now().Unix()),
		Trace:        trace,
	}

	s.batches = append(s.batches, batch)
	s.currentBatch++

	return batch, nil
}

// GetLatestBatch returns the most recent batch
func (s *Sequencer) GetLatestBatch() *RollupBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.batches) == 0 {
		return nil
	}
	return s.batches[len(s.batches)-1]
}

// GetMMR returns the underlying MMR
func (s *Sequencer) GetMMR() *mmr.MerkleMountainRange {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateMMR
}
