// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package types

import (
	"crypto/sha256"
	"encoding/binary"
	"math/big"
)

type Address [20]byte
type Hash [32]byte

func (h Hash) String() string {
	const hex = "0123456789abcdef"
	buf := make([]byte, 64)
	for i, b := range h {
		buf[i*2] = hex[b>>4]
		buf[i*2+1] = hex[b&0x0f]
	}
	return "0x" + string(buf)
}

// Transaction represents an atomic L2 rollup transaction
type Transaction struct {
	Hash     Hash
	Sender   Address
	To       Address
	Nonce    uint64
	Value    *big.Int
	Data     []byte
	GasLimit uint64
	GasPrice *big.Int
}

func NewTransaction(sender Address, to Address, nonce uint64, value *big.Int, data []byte, gasLimit uint64, gasPrice *big.Int) *Transaction {
	tx := &Transaction{
		Sender:   sender,
		To:       to,
		Nonce:    nonce,
		Value:    new(big.Int).Set(value),
		Data:     data,
		GasLimit: gasLimit,
		GasPrice: new(big.Int).Set(gasPrice),
	}
	tx.Hash = tx.ComputeHash()
	return tx
}

func (tx *Transaction) ComputeHash() Hash {
	h := sha256.New()
	h.Write(tx.Sender[:])
	h.Write(tx.To[:])
	var nBuf [8]byte
	binary.BigEndian.PutUint64(nBuf[:], tx.Nonce)
	h.Write(nBuf[:])
	if tx.Value != nil {
		h.Write(tx.Value.Bytes())
	}
	h.Write(tx.Data)
	var gBuf [8]byte
	binary.BigEndian.PutUint64(gBuf[:], tx.GasLimit)
	h.Write(gBuf[:])
	if tx.GasPrice != nil {
		h.Write(tx.GasPrice.Bytes())
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// Bundle represents an atomic MEV searcher bundle
type Bundle struct {
	Transactions []*Transaction
	Bribe        *big.Int
	MinTimestamp uint64
	MaxTimestamp uint64
}

// AccountState represents the account balance and nonce
type AccountState struct {
	Balance *big.Int
	Nonce   uint64
	Storage map[Hash]Hash
}

func NewAccountState() *AccountState {
	return &AccountState{
		Balance: new(big.Int),
		Nonce:   0,
		Storage: make(map[Hash]Hash),
	}
}

func (a *AccountState) Clone() *AccountState {
	cp := &AccountState{
		Balance: new(big.Int).Set(a.Balance),
		Nonce:   a.Nonce,
		Storage: make(map[Hash]Hash, len(a.Storage)),
	}
	for k, v := range a.Storage {
		cp.Storage[k] = v
	}
	return cp
}
