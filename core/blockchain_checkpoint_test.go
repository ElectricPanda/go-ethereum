package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestShouldCommitStateCheckpointInterval(t *testing.T) {
	if !shouldCommitStateCheckpoint(stateCheckpointInterval-1, params.TestChainConfig) {
		t.Fatalf("expected checkpoint commit before interval boundary")
	}
	if shouldCommitStateCheckpoint(stateCheckpointInterval, params.TestChainConfig) {
		t.Fatalf("did not expect checkpoint commit after interval boundary")
	}
}

func TestShouldCommitStateCheckpointPreBlockFork(t *testing.T) {
	config := &params.ChainConfig{
		BerlinBlock: big.NewInt(10),
	}
	if !shouldCommitStateCheckpoint(9, config) {
		t.Fatalf("expected checkpoint commit before block-based fork")
	}
	if shouldCommitStateCheckpoint(10, config) {
		t.Fatalf("did not expect checkpoint commit after block-based fork boundary")
	}
}

func TestShouldCommitParentStateCheckpointTimestampFork(t *testing.T) {
	config := &params.ChainConfig{
		LondonBlock:  big.NewInt(0),
		ShanghaiTime: u64ptr(10),
	}
	parent := &types.Header{
		Number: big.NewInt(9),
		Time:   9,
	}
	current := &types.Header{
		Number: big.NewInt(10),
		Time:   10,
	}
	if !shouldCommitParentStateCheckpoint(parent, current, config) {
		t.Fatalf("expected parent checkpoint commit at timestamp fork boundary")
	}
}

func TestShouldCommitParentStateCheckpointNoTimestampFork(t *testing.T) {
	config := &params.ChainConfig{
		LondonBlock:  big.NewInt(0),
		ShanghaiTime: u64ptr(10),
	}
	parent := &types.Header{
		Number: big.NewInt(9),
		Time:   10,
	}
	current := &types.Header{
		Number: big.NewInt(10),
		Time:   11,
	}
	if shouldCommitParentStateCheckpoint(parent, current, config) {
		t.Fatalf("did not expect parent checkpoint commit once timestamp fork is already active")
	}
}

func u64ptr(v uint64) *uint64 {
	return &v
}
