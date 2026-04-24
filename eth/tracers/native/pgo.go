// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package native

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/params"
)

func init() {
	tracers.DefaultDirectory.Register("pgoTracer", newPGOTracer, false)
}

type pgoTracerConfig struct {
	OnlyTopCall           bool     `json:"onlyTopCall"`
	IncludeTxSummary      bool     `json:"includeTxSummary"`
	IncludeCallFrames     bool     `json:"includeCallFrames"`
	IncludeProfiles       bool     `json:"includeProfiles"`
	IncludeRawPayloads    bool     `json:"includeRawPayloads"`
	IncludeLogPayloads    bool     `json:"includeLogPayloads"`
	IncludeFaultSummaries bool     `json:"includeFaultSummaries"`
	FocusAddresses        []string `json:"focusAddresses,omitempty"`
	FocusSelectors        []string `json:"focusSelectors,omitempty"`
}

type pgoTxSummary struct {
	BlockHash               common.Hash                       `json:"blockHash"`
	BlockNumber             hexutil.Uint64                    `json:"blockNumber"`
	TxHash                  common.Hash                       `json:"txHash"`
	TxIndex                 int                               `json:"txIndex"`
	From                    common.Address                    `json:"from"`
	To                      *common.Address                   `json:"to,omitempty"`
	GasLimit                hexutil.Uint64                    `json:"gasLimit"`
	GasUsed                 hexutil.Uint64                    `json:"gasUsed"`
	EffectiveGasPrice       *hexutil.Big                      `json:"effectiveGasPrice,omitempty"`
	Value                   *hexutil.Big                      `json:"value,omitempty"`
	Status                  *hexutil.Uint64                   `json:"status,omitempty"`
	TopSelector             string                            `json:"topSelector,omitempty"`
	TopCallType             string                            `json:"topCallType,omitempty"`
	TopCallReverted         bool                              `json:"topCallReverted"`
	TopCallError            string                            `json:"topCallError,omitempty"`
	TopCalldataSize         int                               `json:"topCalldataSize"`
	TopInputSize            int                               `json:"topInputSize"`
	TopOutputSize           int                               `json:"topOutputSize"`
	CallCount               int                               `json:"callCount"`
	LogCount                int                               `json:"logCount"`
	CreateCount             int                               `json:"createCount"`
	SelfdestructCount       int                               `json:"selfdestructCount"`
	PrecompileCallCount     int                               `json:"precompileCallCount"`
	PrecompileFrameCount    int                               `json:"precompileFrameCount"`
	TouchedAccountsCount    int                               `json:"touchedAccountsCount"`
	ExecutedCode            bool                              `json:"executedCode"`
	ExecutedFrameCount      int                               `json:"executedFrameCount"`
	GasChangeCount          int                               `json:"gasChangeCount"`
	BlockHashReadCount      int                               `json:"blockHashReadCount"`
	TouchedAccounts         []string                          `json:"touchedAccounts,omitempty"`
	CreatedAccounts         []string                          `json:"createdAccounts,omitempty"`
	DeletedAccounts         []string                          `json:"deletedAccounts,omitempty"`
	CodeChangedAccounts     []string                          `json:"codeChangedAccounts,omitempty"`
	StorageTouchCounts      map[string]uint64                 `json:"storageTouchCounts,omitempty"`
	StorageSlotTouchCounts  map[string]map[string]uint64      `json:"storageSlotTouchCounts,omitempty"`
	BalanceChangeCounts     map[string]uint64                 `json:"balanceChangeCounts,omitempty"`
	BalanceChangeReasons    map[string]uint64                 `json:"balanceChangeReasons,omitempty"`
	BalanceDiffs            map[string]pgoBigDiff             `json:"balanceDiffs,omitempty"`
	NonceChangeCounts       map[string]uint64                 `json:"nonceChangeCounts,omitempty"`
	NonceChangeReasons      map[string]uint64                 `json:"nonceChangeReasons,omitempty"`
	NonceDiffs              map[string]pgoUint64Diff          `json:"nonceDiffs,omitempty"`
	CodeChangeCounts        map[string]uint64                 `json:"codeChangeCounts,omitempty"`
	CodeChangeReasons       map[string]uint64                 `json:"codeChangeReasons,omitempty"`
	CodeDiffs               map[string]pgoCodeDiff            `json:"codeDiffs,omitempty"`
	StorageDiffs            map[string]map[string]pgoHashDiff `json:"storageDiffs,omitempty"`
	GasChangeReasons        map[string]uint64                 `json:"gasChangeReasons,omitempty"`
	GasChangeDebitByReason  map[string]uint64                 `json:"gasChangeDebitByReason,omitempty"`
	GasChangeCreditByReason map[string]uint64                 `json:"gasChangeCreditByReason,omitempty"`
	GasChangeNetByReason    map[string]string                 `json:"gasChangeNetByReason,omitempty"`
	BlockHashReads          map[string]string                 `json:"blockHashReads,omitempty"`
}

type pgoBigDiff struct {
	Prev string `json:"prev"`
	New  string `json:"new"`
}

type pgoUint64Diff struct {
	Prev hexutil.Uint64 `json:"prev"`
	New  hexutil.Uint64 `json:"new"`
}

type pgoHashDiff struct {
	Prev string `json:"prev"`
	New  string `json:"new"`
}

type pgoCodeDiff struct {
	PrevHash    string `json:"prevHash,omitempty"`
	NewHash     string `json:"newHash,omitempty"`
	PrevPresent bool   `json:"prevPresent"`
	NewPresent  bool   `json:"newPresent"`
}

type pgoCallFrameSummary struct {
	FrameID        int               `json:"frameId"`
	ParentFrameID  *int              `json:"parentFrameId,omitempty"`
	Depth          int               `json:"depth"`
	CallType       string            `json:"callType"`
	From           common.Address    `json:"from"`
	To             *common.Address   `json:"to,omitempty"`
	ContextAddress *common.Address   `json:"contextAddress,omitempty"`
	InputSelector  string            `json:"inputSelector,omitempty"`
	InputSize      int               `json:"inputSize"`
	InputData      string            `json:"inputData,omitempty"`
	OutputSize     int               `json:"outputSize"`
	OutputData     string            `json:"outputData,omitempty"`
	GasIn          hexutil.Uint64    `json:"gasIn"`
	GasUsed        hexutil.Uint64    `json:"gasUsed"`
	Value          *hexutil.Big      `json:"value,omitempty"`
	Reverted       bool              `json:"reverted"`
	Error          string            `json:"error,omitempty"`
	LogsCount      int               `json:"logsCount"`
	IsPrecompile   bool              `json:"isPrecompile"`
	PrecompileName string            `json:"precompileName,omitempty"`
	ExecutedCode   bool              `json:"executedCode"`
	CodeHashKeccak *common.Hash      `json:"codeHashKeccak,omitempty"`
	CodeHashSha256 string            `json:"codeHashSha256,omitempty"`
	Logs           []pgoLogSummary   `json:"logs,omitempty"`
	Faults         []pgoFaultSummary `json:"faults,omitempty"`
}

type pgoLogSummary struct {
	Address common.Address `json:"address"`
	Topics  []string       `json:"topics,omitempty"`
	Data    string         `json:"data,omitempty"`
}

type pgoFaultSummary struct {
	PC    hexutil.Uint64 `json:"pc"`
	Op    string         `json:"op"`
	Gas   hexutil.Uint64 `json:"gas"`
	Cost  hexutil.Uint64 `json:"cost"`
	Error string         `json:"error"`
}

type pgoCodeProfile struct {
	FrameID           int               `json:"frameId"`
	CodeHashKeccak    *common.Hash      `json:"codeHashKeccak,omitempty"`
	CodeHashSha256    string            `json:"codeHashSha256,omitempty"`
	ContextAddress    *common.Address   `json:"contextAddress,omitempty"`
	PCCounts          map[string]uint64 `json:"pcCounts,omitempty"`
	PCGas             map[string]uint64 `json:"pcGas,omitempty"`
	EdgeCounts        map[string]uint64 `json:"edgeCounts,omitempty"`
	EdgeGas           map[string]uint64 `json:"edgeGas,omitempty"`
	OpcodeCounts      map[string]uint64 `json:"opcodeCounts,omitempty"`
	SLOADCount        uint64            `json:"sloadCount,omitempty"`
	SSTORECount       uint64            `json:"sstoreCount,omitempty"`
	EXTCodeCount      uint64            `json:"extcodeCount,omitempty"`
	CallOpcodeCount   uint64            `json:"callOpcodeCount,omitempty"`
	LogOpcodeCount    uint64            `json:"logOpcodeCount,omitempty"`
	RevertOpcodeCount uint64            `json:"revertOpcodeCount,omitempty"`
}

type pgoTraceResult struct {
	TraceSchemaVersion string                `json:"traceSchemaVersion"`
	Tx                 *pgoTxSummary         `json:"tx,omitempty"`
	Frames             []pgoCallFrameSummary `json:"frames,omitempty"`
	Profiles           []pgoCodeProfile      `json:"profiles,omitempty"`
}

type pgoFrameState struct {
	summary  pgoCallFrameSummary
	profile  pgoCodeProfile
	lastPC   uint64
	hasPC    bool
	selected bool
}

type pgoAccountLifecycle struct {
	HasNonce                bool
	InitialNonce            uint64
	FinalNonce              uint64
	SawNewContractNonce     bool
	HasCode                 bool
	InitialCodePresent      bool
	FinalCodePresent        bool
	SawContractCreationCode bool
}

type pgoBigDiffState struct {
	Prev *big.Int
	New  *big.Int
}

type pgoUint64DiffState struct {
	Prev uint64
	New  uint64
}

type pgoHashDiffState struct {
	Prev common.Hash
	New  common.Hash
}

type pgoCodeDiffState struct {
	PrevHash    common.Hash
	NewHash     common.Hash
	PrevPresent bool
	NewPresent  bool
}

type pgoTracer struct {
	config                  pgoTracerConfig
	chainCfg                *params.ChainConfig
	precompiles             vm.PrecompiledContracts
	txctx                   *tracers.Context
	tx                      pgoTxSummary
	frames                  []*pgoFrameState
	frameByID               map[int]*pgoFrameState
	stack                   []int
	nextID                  int
	touched                 map[common.Address]struct{}
	created                 map[common.Address]struct{}
	deleted                 map[common.Address]struct{}
	codeChanged             map[common.Address]struct{}
	storageTouchCounts      map[common.Address]uint64
	storageSlotTouchCounts  map[common.Address]map[common.Hash]uint64
	balanceChangeCounts     map[common.Address]uint64
	balanceChangeReasons    map[string]uint64
	balanceDiffs            map[common.Address]*pgoBigDiffState
	nonceChangeCounts       map[common.Address]uint64
	nonceChangeReasons      map[string]uint64
	nonceDiffs              map[common.Address]*pgoUint64DiffState
	codeChangeCounts        map[common.Address]uint64
	codeChangeReasons       map[string]uint64
	codeDiffs               map[common.Address]*pgoCodeDiffState
	gasChangeReasons        map[string]uint64
	gasChangeDebitByReason  map[string]uint64
	gasChangeCreditByReason map[string]uint64
	blockHashReads          map[uint64]common.Hash
	accountLifecycle        map[common.Address]*pgoAccountLifecycle
	storageDiffs            map[common.Address]map[common.Hash]*pgoHashDiffState
	focusAddresses          map[common.Address]struct{}
	focusSelectors          map[string]struct{}
	interrupt               atomic.Bool
	reason                  error
}

func newPGOTracer(ctx *tracers.Context, cfg json.RawMessage, chainConfig *params.ChainConfig) (*tracers.Tracer, error) {
	var config pgoTracerConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		return nil, err
	}
	// Default to all sections enabled unless explicitly disabled.
	if len(cfg) == 0 || string(cfg) == "{}" ||
		(!config.IncludeTxSummary && !config.IncludeCallFrames && !config.IncludeProfiles) {
		config.IncludeTxSummary = true
		config.IncludeCallFrames = true
		config.IncludeProfiles = true
	}
	t := &pgoTracer{
		config:                  config,
		chainCfg:                chainConfig,
		txctx:                   ctx,
		frameByID:               make(map[int]*pgoFrameState),
		touched:                 make(map[common.Address]struct{}),
		created:                 make(map[common.Address]struct{}),
		deleted:                 make(map[common.Address]struct{}),
		codeChanged:             make(map[common.Address]struct{}),
		storageTouchCounts:      make(map[common.Address]uint64),
		storageSlotTouchCounts:  make(map[common.Address]map[common.Hash]uint64),
		balanceChangeCounts:     make(map[common.Address]uint64),
		balanceChangeReasons:    make(map[string]uint64),
		balanceDiffs:            make(map[common.Address]*pgoBigDiffState),
		nonceChangeCounts:       make(map[common.Address]uint64),
		nonceChangeReasons:      make(map[string]uint64),
		nonceDiffs:              make(map[common.Address]*pgoUint64DiffState),
		codeChangeCounts:        make(map[common.Address]uint64),
		codeChangeReasons:       make(map[string]uint64),
		codeDiffs:               make(map[common.Address]*pgoCodeDiffState),
		gasChangeReasons:        make(map[string]uint64),
		gasChangeDebitByReason:  make(map[string]uint64),
		gasChangeCreditByReason: make(map[string]uint64),
		blockHashReads:          make(map[uint64]common.Hash),
		accountLifecycle:        make(map[common.Address]*pgoAccountLifecycle),
		storageDiffs:            make(map[common.Address]map[common.Hash]*pgoHashDiffState),
		focusAddresses:          make(map[common.Address]struct{}),
		focusSelectors:          make(map[string]struct{}),
	}
	for _, raw := range config.FocusAddresses {
		if common.IsHexAddress(raw) {
			t.focusAddresses[common.HexToAddress(raw)] = struct{}{}
		}
	}
	for _, raw := range config.FocusSelectors {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if len(normalized) == 10 && strings.HasPrefix(normalized, "0x") {
			t.focusSelectors[normalized] = struct{}{}
		}
	}
	hooks, err := tracing.WrapWithJournal(&tracing.Hooks{
		OnTxStart:       t.OnTxStart,
		OnTxEnd:         t.OnTxEnd,
		OnEnter:         t.OnEnter,
		OnExit:          t.OnExit,
		OnOpcode:        t.OnOpcode,
		OnFault:         t.OnFault,
		OnGasChange:     t.OnGasChange,
		OnBalanceChange: t.OnBalanceChange,
		OnNonceChangeV2: t.OnNonceChangeV2,
		OnCodeChangeV2:  t.OnCodeChangeV2,
		OnStorageChange: t.OnStorageChange,
		OnLog:           t.OnLog,
		OnBlockHashRead: t.OnBlockHashRead,
	})
	if err != nil {
		return nil, err
	}
	return &tracers.Tracer{
		Hooks:     hooks,
		GetResult: t.GetResult,
		Stop:      t.Stop,
	}, nil
}

func selectorFromInput(input []byte) string {
	if len(input) < 4 {
		return ""
	}
	return hexutil.Encode(input[:4])
}

func (t *pgoTracer) hasFocusAddressFilter() bool {
	return len(t.focusAddresses) > 0
}

func (t *pgoTracer) hasFocusSelectorFilter() bool {
	return len(t.focusSelectors) > 0
}

func (t *pgoTracer) matchesFocusAddress(addr common.Address) bool {
	if len(t.focusAddresses) == 0 {
		return false
	}
	_, ok := t.focusAddresses[addr]
	return ok
}

func (t *pgoTracer) matchesFocusSelector(selector string) bool {
	if len(t.focusSelectors) == 0 {
		return false
	}
	_, ok := t.focusSelectors[strings.ToLower(selector)]
	return ok
}

func (t *pgoTracer) txPassesFocusFilters() bool {
	if !t.hasFocusAddressFilter() && !t.hasFocusSelectorFilter() {
		return true
	}
	if t.hasFocusSelectorFilter() && !t.matchesFocusSelector(t.tx.TopSelector) {
		return false
	}
	if t.hasFocusAddressFilter() {
		for _, frame := range t.frames {
			if frame.selected {
				return true
			}
		}
		return false
	}
	return true
}

func sha256Hex(code []byte) string {
	sum := sha256.Sum256(code)
	return hexutil.Encode(sum[:])
}

func addressKey(addr common.Address) string {
	return strings.ToLower(addr.Hex())
}

func hashKey(hash common.Hash) string {
	return strings.ToLower(hash.Hex())
}

func cloneBigInt(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func bigToHex(v *big.Int) string {
	if v == nil {
		return "0x0"
	}
	return hexutil.EncodeBig(v)
}

func codePresent(codeHash common.Hash, code []byte) bool {
	if len(code) > 0 {
		return true
	}
	return codeHash != (common.Hash{}) && codeHash != types.EmptyCodeHash
}

func (t *pgoTracer) touch(addr common.Address) {
	t.touched[addr] = struct{}{}
}

func (t *pgoTracer) lifecycle(addr common.Address) *pgoAccountLifecycle {
	lc, ok := t.accountLifecycle[addr]
	if !ok {
		lc = &pgoAccountLifecycle{}
		t.accountLifecycle[addr] = lc
	}
	return lc
}

func bumpAddressCount(m map[common.Address]uint64, addr common.Address) {
	m[addr]++
}

func bumpStringCount(m map[string]uint64, key string) {
	m[key]++
}

func cloneAddressCounts(m map[common.Address]uint64) map[string]uint64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]uint64, len(m))
	for addr, count := range m {
		out[addressKey(addr)] = count
	}
	return out
}

func cloneNestedSlotCounts(m map[common.Address]map[common.Hash]uint64) map[string]map[string]uint64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]map[string]uint64, len(m))
	for addr, slots := range m {
		addrOut := make(map[string]uint64, len(slots))
		for slot, count := range slots {
			addrOut[hashKey(slot)] = count
		}
		out[addressKey(addr)] = addrOut
	}
	return out
}

func cloneStringCounts(m map[string]uint64) map[string]uint64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]uint64, len(m))
	for key, count := range m {
		out[key] = count
	}
	return out
}

func cloneBigDiffs(m map[common.Address]*pgoBigDiffState) map[string]pgoBigDiff {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]pgoBigDiff, len(m))
	for addr, diff := range m {
		if diff == nil {
			continue
		}
		out[addressKey(addr)] = pgoBigDiff{
			Prev: bigToHex(diff.Prev),
			New:  bigToHex(diff.New),
		}
	}
	return out
}

func cloneUint64Diffs(m map[common.Address]*pgoUint64DiffState) map[string]pgoUint64Diff {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]pgoUint64Diff, len(m))
	for addr, diff := range m {
		if diff == nil {
			continue
		}
		out[addressKey(addr)] = pgoUint64Diff{
			Prev: hexutil.Uint64(diff.Prev),
			New:  hexutil.Uint64(diff.New),
		}
	}
	return out
}

func cloneCodeDiffs(m map[common.Address]*pgoCodeDiffState) map[string]pgoCodeDiff {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]pgoCodeDiff, len(m))
	for addr, diff := range m {
		if diff == nil {
			continue
		}
		entry := pgoCodeDiff{
			PrevPresent: diff.PrevPresent,
			NewPresent:  diff.NewPresent,
		}
		if diff.PrevPresent || diff.PrevHash != (common.Hash{}) {
			entry.PrevHash = hashKey(diff.PrevHash)
		}
		if diff.NewPresent || diff.NewHash != (common.Hash{}) {
			entry.NewHash = hashKey(diff.NewHash)
		}
		out[addressKey(addr)] = entry
	}
	return out
}

func cloneStorageDiffs(m map[common.Address]map[common.Hash]*pgoHashDiffState) map[string]map[string]pgoHashDiff {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]map[string]pgoHashDiff, len(m))
	for addr, slots := range m {
		addrOut := make(map[string]pgoHashDiff, len(slots))
		for slot, diff := range slots {
			if diff == nil {
				continue
			}
			addrOut[hashKey(slot)] = pgoHashDiff{
				Prev: hashKey(diff.Prev),
				New:  hashKey(diff.New),
			}
		}
		out[addressKey(addr)] = addrOut
	}
	return out
}

func cloneGasNet(debit, credit map[string]uint64) map[string]string {
	if len(debit) == 0 && len(credit) == 0 {
		return nil
	}
	keys := make(map[string]struct{}, len(debit)+len(credit))
	for key := range debit {
		keys[key] = struct{}{}
	}
	for key := range credit {
		keys[key] = struct{}{}
	}
	out := make(map[string]string, len(keys))
	for key := range keys {
		debitBig := new(big.Int).SetUint64(debit[key])
		creditBig := new(big.Int).SetUint64(credit[key])
		out[key] = new(big.Int).Sub(creditBig, debitBig).String()
	}
	return out
}

func sortedAddressList(m map[common.Address]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for addr := range m {
		out = append(out, addressKey(addr))
	}
	sort.Strings(out)
	return out
}

func cloneBlockHashReads(m map[uint64]common.Hash) map[string]string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]uint64, 0, len(m))
	for num := range m {
		keys = append(keys, num)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make(map[string]string, len(m))
	for _, num := range keys {
		out[fmt.Sprintf("%d", num)] = hashKey(m[num])
	}
	return out
}

func (t *pgoTracer) currentFrame() *pgoFrameState {
	if len(t.stack) == 0 {
		return nil
	}
	return t.frameByID[t.stack[len(t.stack)-1]]
}

func (t *pgoTracer) ensureCodeMetadata(frame *pgoFrameState, scope tracing.OpContext) {
	if frame == nil {
		return
	}
	if frame.summary.ContextAddress == nil {
		addr := scope.Address()
		frame.summary.ContextAddress = &addr
		frame.profile.ContextAddress = &addr
		t.touch(addr)
	}
	if frame.summary.CodeHashKeccak != nil && frame.summary.CodeHashSha256 != "" {
		return
	}
	code := scope.ContractCode()
	if len(code) == 0 {
		return
	}
	keccak := crypto.Keccak256Hash(code)
	frame.summary.CodeHashKeccak = &keccak
	frame.profile.CodeHashKeccak = &keccak
	frame.summary.CodeHashSha256 = sha256Hex(code)
	frame.profile.CodeHashSha256 = frame.summary.CodeHashSha256
}

func (t *pgoTracer) OnTxStart(env *tracing.VMContext, tx *types.Transaction, from common.Address) {
	t.tx = pgoTxSummary{}
	if t.chainCfg != nil && env != nil && env.BlockNumber != nil {
		rules := t.chainCfg.Rules(env.BlockNumber, env.Random != nil, env.Time)
		t.precompiles = vm.ActivePrecompiledContracts(rules)
	} else {
		t.precompiles = nil
	}
	if t.txctx != nil {
		t.tx.BlockHash = t.txctx.BlockHash
		if t.txctx.BlockNumber != nil {
			t.tx.BlockNumber = hexutil.Uint64(t.txctx.BlockNumber.Uint64())
		}
		t.tx.TxHash = t.txctx.TxHash
		t.tx.TxIndex = t.txctx.TxIndex
	} else {
		t.tx.TxHash = tx.Hash()
	}
	t.tx.From = from
	if to := tx.To(); to != nil {
		toCopy := *to
		t.tx.To = &toCopy
		t.touch(toCopy)
	}
	t.touch(from)
	t.tx.GasLimit = hexutil.Uint64(tx.Gas())
	if gasPrice := tx.GasPrice(); gasPrice != nil {
		t.tx.EffectiveGasPrice = (*hexutil.Big)(new(big.Int).Set(gasPrice))
	}
	if value := tx.Value(); value != nil {
		t.tx.Value = (*hexutil.Big)(new(big.Int).Set(value))
	}
	input := tx.Data()
	t.tx.TopSelector = selectorFromInput(input)
	t.tx.TopCalldataSize = len(input)
	t.tx.TopInputSize = len(input)
	t.frames = nil
	t.stack = nil
	t.nextID = 0
	t.frameByID = make(map[int]*pgoFrameState)
	t.touched = make(map[common.Address]struct{})
	t.created = make(map[common.Address]struct{})
	t.deleted = make(map[common.Address]struct{})
	t.codeChanged = make(map[common.Address]struct{})
	t.storageTouchCounts = make(map[common.Address]uint64)
	t.storageSlotTouchCounts = make(map[common.Address]map[common.Hash]uint64)
	t.balanceChangeCounts = make(map[common.Address]uint64)
	t.balanceChangeReasons = make(map[string]uint64)
	t.balanceDiffs = make(map[common.Address]*pgoBigDiffState)
	t.nonceChangeCounts = make(map[common.Address]uint64)
	t.nonceChangeReasons = make(map[string]uint64)
	t.nonceDiffs = make(map[common.Address]*pgoUint64DiffState)
	t.codeChangeCounts = make(map[common.Address]uint64)
	t.codeChangeReasons = make(map[string]uint64)
	t.codeDiffs = make(map[common.Address]*pgoCodeDiffState)
	t.gasChangeReasons = make(map[string]uint64)
	t.gasChangeDebitByReason = make(map[string]uint64)
	t.gasChangeCreditByReason = make(map[string]uint64)
	t.blockHashReads = make(map[uint64]common.Hash)
	t.accountLifecycle = make(map[common.Address]*pgoAccountLifecycle)
	t.storageDiffs = make(map[common.Address]map[common.Hash]*pgoHashDiffState)
	t.touch(from)
	if t.tx.To != nil {
		t.touch(*t.tx.To)
	}
	if env != nil {
		t.touch(env.Coinbase)
	}
}

func (t *pgoTracer) OnEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.interrupt.Load() {
		return
	}
	if t.config.OnlyTopCall && depth > 0 {
		return
	}
	frameID := t.nextID
	t.nextID++

	var parentID *int
	if len(t.stack) > 0 {
		pid := t.stack[len(t.stack)-1]
		parentID = &pid
	}
	toCopy := to
	frame := &pgoFrameState{
		summary: pgoCallFrameSummary{
			FrameID:       frameID,
			ParentFrameID: parentID,
			Depth:         depth,
			CallType:      vm.OpCode(typ).String(),
			From:          from,
			To:            &toCopy,
			InputSelector: selectorFromInput(input),
			InputSize:     len(input),
			GasIn:         hexutil.Uint64(gas),
		},
		profile: pgoCodeProfile{
			FrameID:      frameID,
			PCCounts:     make(map[string]uint64),
			PCGas:        make(map[string]uint64),
			EdgeCounts:   make(map[string]uint64),
			EdgeGas:      make(map[string]uint64),
			OpcodeCounts: make(map[string]uint64),
		},
	}
	if t.config.IncludeRawPayloads && len(input) > 0 {
		frame.summary.InputData = hexutil.Encode(input)
	}
	if precompile, ok := t.precompiles[to]; ok {
		frame.summary.IsPrecompile = true
		frame.summary.PrecompileName = precompile.Name()
		t.tx.PrecompileCallCount++
	}
	if value != nil {
		frame.summary.Value = (*hexutil.Big)(new(big.Int).Set(value))
	}
	if t.matchesFocusAddress(to) {
		frame.selected = true
	}
	t.frames = append(t.frames, frame)
	t.frameByID[frameID] = frame
	t.stack = append(t.stack, frameID)
	t.tx.CallCount++
	if frame.summary.CallType == vm.CREATE.String() || frame.summary.CallType == vm.CREATE2.String() {
		t.tx.CreateCount++
	}
	t.touch(from)
	t.touch(to)
}

func (t *pgoTracer) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if t.config.OnlyTopCall && depth > 0 {
		return
	}
	if len(t.stack) == 0 {
		return
	}
	frame := t.currentFrame()
	if frame == nil {
		return
	}
	frame.summary.OutputSize = len(output)
	if t.config.IncludeRawPayloads && len(output) > 0 {
		frame.summary.OutputData = hexutil.Encode(output)
	}
	frame.summary.GasUsed = hexutil.Uint64(gasUsed)
	frame.summary.Reverted = reverted
	if err != nil {
		frame.summary.Error = err.Error()
	}
	t.stack = t.stack[:len(t.stack)-1]
}

func (t *pgoTracer) OnFault(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, depth int, err error) {
	if err == nil || t.interrupt.Load() {
		return
	}
	if t.config.OnlyTopCall && depth > 1 {
		return
	}
	if !t.config.IncludeFaultSummaries {
		return
	}
	frame := t.currentFrame()
	if frame == nil {
		return
	}
	frame.summary.Faults = append(frame.summary.Faults, pgoFaultSummary{
		PC:    hexutil.Uint64(pc),
		Op:    vm.OpCode(op).String(),
		Gas:   hexutil.Uint64(gas),
		Cost:  hexutil.Uint64(cost),
		Error: err.Error(),
	})
}

func (t *pgoTracer) OnOpcode(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if err != nil || t.interrupt.Load() {
		return
	}
	// OnEnter/OnExit report the top-level call at depth=0, while OnOpcode uses a
	// one-based depth where the top-level call executes at depth=1.
	if t.config.OnlyTopCall && depth > 1 {
		return
	}
	frame := t.currentFrame()
	if frame == nil {
		return
	}
	frame.summary.ExecutedCode = true
	t.ensureCodeMetadata(frame, scope)
	if frame.summary.ContextAddress != nil && t.matchesFocusAddress(*frame.summary.ContextAddress) {
		frame.selected = true
	}

	pcKey := fmt.Sprintf("%d", pc)
	frame.profile.PCCounts[pcKey]++
	frame.profile.PCGas[pcKey] += cost

	if frame.hasPC {
		edgeKey := fmt.Sprintf("%d->%d", frame.lastPC, pc)
		frame.profile.EdgeCounts[edgeKey]++
		frame.profile.EdgeGas[edgeKey] += cost
	}
	frame.lastPC = pc
	frame.hasPC = true

	opName := vm.OpCode(op).String()
	frame.profile.OpcodeCounts[opName]++

	switch vm.OpCode(op) {
	case vm.SLOAD:
		frame.profile.SLOADCount++
	case vm.SSTORE:
		frame.profile.SSTORECount++
	case vm.EXTCODECOPY, vm.EXTCODEHASH, vm.EXTCODESIZE:
		frame.profile.EXTCodeCount++
	case vm.CALL, vm.CALLCODE, vm.DELEGATECALL, vm.STATICCALL:
		frame.profile.CallOpcodeCount++
	case vm.LOG0, vm.LOG1, vm.LOG2, vm.LOG3, vm.LOG4:
		frame.profile.LogOpcodeCount++
	case vm.REVERT:
		frame.profile.RevertOpcodeCount++
	case vm.SELFDESTRUCT:
		t.tx.SelfdestructCount++
		stack := scope.StackData()
		if len(stack) >= 1 {
			t.touch(common.Address(stack[len(stack)-1].Bytes20()))
		}
	}
}

func (t *pgoTracer) OnGasChange(old, new uint64, reason tracing.GasChangeReason) {
	if t.interrupt.Load() {
		return
	}
	t.tx.GasChangeCount++
	reasonKey := reason.String()
	bumpStringCount(t.gasChangeReasons, reasonKey)
	if old > new {
		t.gasChangeDebitByReason[reasonKey] += old - new
	} else if new > old {
		t.gasChangeCreditByReason[reasonKey] += new - old
	}
}

func (t *pgoTracer) OnBalanceChange(addr common.Address, prev, new *big.Int, reason tracing.BalanceChangeReason) {
	if t.interrupt.Load() {
		return
	}
	t.touch(addr)
	bumpAddressCount(t.balanceChangeCounts, addr)
	bumpStringCount(t.balanceChangeReasons, reason.String())
	if _, ok := t.balanceDiffs[addr]; !ok {
		t.balanceDiffs[addr] = &pgoBigDiffState{Prev: cloneBigInt(prev)}
	}
	t.balanceDiffs[addr].New = cloneBigInt(new)
}

func (t *pgoTracer) OnNonceChangeV2(addr common.Address, prev, new uint64, reason tracing.NonceChangeReason) {
	if t.interrupt.Load() {
		return
	}
	t.touch(addr)
	bumpAddressCount(t.nonceChangeCounts, addr)
	bumpStringCount(t.nonceChangeReasons, reason.String())
	if _, ok := t.nonceDiffs[addr]; !ok {
		t.nonceDiffs[addr] = &pgoUint64DiffState{Prev: prev}
	}
	t.nonceDiffs[addr].New = new
	lc := t.lifecycle(addr)
	if !lc.HasNonce {
		lc.InitialNonce = prev
		lc.HasNonce = true
	}
	lc.FinalNonce = new
	if reason == tracing.NonceChangeNewContract {
		lc.SawNewContractNonce = true
	}
}

func (t *pgoTracer) OnCodeChangeV2(addr common.Address, prevCodeHash common.Hash, prevCode []byte, codeHash common.Hash, code []byte, reason tracing.CodeChangeReason) {
	if t.interrupt.Load() {
		return
	}
	t.touch(addr)
	t.codeChanged[addr] = struct{}{}
	bumpAddressCount(t.codeChangeCounts, addr)
	bumpStringCount(t.codeChangeReasons, reason.String())
	if _, ok := t.codeDiffs[addr]; !ok {
		t.codeDiffs[addr] = &pgoCodeDiffState{
			PrevHash:    prevCodeHash,
			PrevPresent: codePresent(prevCodeHash, prevCode),
		}
	}
	t.codeDiffs[addr].NewHash = codeHash
	t.codeDiffs[addr].NewPresent = codePresent(codeHash, code)
	lc := t.lifecycle(addr)
	if !lc.HasCode {
		lc.InitialCodePresent = codePresent(prevCodeHash, prevCode)
		lc.HasCode = true
	}
	lc.FinalCodePresent = codePresent(codeHash, code)
	if reason == tracing.CodeChangeContractCreation {
		lc.SawContractCreationCode = true
	}
}

func (t *pgoTracer) OnStorageChange(addr common.Address, slot common.Hash, prev, new common.Hash) {
	if t.interrupt.Load() {
		return
	}
	t.touch(addr)
	t.storageTouchCounts[addr]++
	if _, ok := t.storageSlotTouchCounts[addr]; !ok {
		t.storageSlotTouchCounts[addr] = make(map[common.Hash]uint64)
	}
	t.storageSlotTouchCounts[addr][slot]++
	if _, ok := t.storageDiffs[addr]; !ok {
		t.storageDiffs[addr] = make(map[common.Hash]*pgoHashDiffState)
	}
	if _, ok := t.storageDiffs[addr][slot]; !ok {
		t.storageDiffs[addr][slot] = &pgoHashDiffState{Prev: prev}
	}
	t.storageDiffs[addr][slot].New = new
}

func (t *pgoTracer) OnLog(log *types.Log) {
	if t.interrupt.Load() {
		return
	}
	t.tx.LogCount++
	t.touch(log.Address)
	frame := t.currentFrame()
	if frame == nil {
		return
	}
	frame.summary.LogsCount++
	if t.config.IncludeLogPayloads {
		topics := make([]string, 0, len(log.Topics))
		for _, topic := range log.Topics {
			topics = append(topics, hashKey(topic))
		}
		frame.summary.Logs = append(frame.summary.Logs, pgoLogSummary{
			Address: log.Address,
			Topics:  topics,
			Data:    hexutil.Encode(log.Data),
		})
	}
}

func (t *pgoTracer) OnBlockHashRead(blockNumber uint64, hash common.Hash) {
	if t.interrupt.Load() {
		return
	}
	t.tx.BlockHashReadCount++
	t.blockHashReads[blockNumber] = hash
}

func (t *pgoTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if receipt != nil {
		t.tx.GasUsed = hexutil.Uint64(receipt.GasUsed)
		if receipt.EffectiveGasPrice != nil {
			t.tx.EffectiveGasPrice = (*hexutil.Big)(new(big.Int).Set(receipt.EffectiveGasPrice))
		}
		status := hexutil.Uint64(receipt.Status)
		t.tx.Status = &status
	}
	t.tx.TouchedAccountsCount = len(t.touched)
	for addr, lc := range t.accountLifecycle {
		if (lc.SawNewContractNonce || lc.SawContractCreationCode) && (lc.FinalNonce > 0 || lc.FinalCodePresent) {
			t.created[addr] = struct{}{}
		}
		if lc.HasCode && lc.InitialCodePresent && !lc.FinalCodePresent {
			t.deleted[addr] = struct{}{}
		}
	}
}

func (t *pgoTracer) GetResult() (json.RawMessage, error) {
	if !t.txPassesFocusFilters() {
		return nil, nil
	}
	result := pgoTraceResult{
		TraceSchemaVersion: "pgo-v2",
	}
	selectedFrames := t.frames
	if t.hasFocusAddressFilter() {
		selectedFrames = make([]*pgoFrameState, 0, len(t.frames))
		for _, frame := range t.frames {
			if frame.selected {
				selectedFrames = append(selectedFrames, frame)
			}
		}
	}
	if t.config.IncludeTxSummary {
		txCopy := t.tx
		for _, frame := range t.frames {
			if frame.summary.IsPrecompile {
				txCopy.PrecompileFrameCount++
			}
			if frame.summary.ExecutedCode {
				txCopy.ExecutedCode = true
				txCopy.ExecutedFrameCount++
			}
			if frame.summary.Depth == 0 {
				txCopy.TopCallType = frame.summary.CallType
				txCopy.TopCallReverted = frame.summary.Reverted
				txCopy.TopCallError = frame.summary.Error
				txCopy.TopOutputSize = frame.summary.OutputSize
			}
		}
		txCopy.TouchedAccounts = sortedAddressList(t.touched)
		txCopy.CreatedAccounts = sortedAddressList(t.created)
		txCopy.DeletedAccounts = sortedAddressList(t.deleted)
		txCopy.CodeChangedAccounts = sortedAddressList(t.codeChanged)
		txCopy.StorageTouchCounts = cloneAddressCounts(t.storageTouchCounts)
		txCopy.StorageSlotTouchCounts = cloneNestedSlotCounts(t.storageSlotTouchCounts)
		txCopy.BalanceChangeCounts = cloneAddressCounts(t.balanceChangeCounts)
		txCopy.BalanceChangeReasons = cloneStringCounts(t.balanceChangeReasons)
		txCopy.BalanceDiffs = cloneBigDiffs(t.balanceDiffs)
		txCopy.NonceChangeCounts = cloneAddressCounts(t.nonceChangeCounts)
		txCopy.NonceChangeReasons = cloneStringCounts(t.nonceChangeReasons)
		txCopy.NonceDiffs = cloneUint64Diffs(t.nonceDiffs)
		txCopy.CodeChangeCounts = cloneAddressCounts(t.codeChangeCounts)
		txCopy.CodeChangeReasons = cloneStringCounts(t.codeChangeReasons)
		txCopy.CodeDiffs = cloneCodeDiffs(t.codeDiffs)
		txCopy.StorageDiffs = cloneStorageDiffs(t.storageDiffs)
		txCopy.GasChangeReasons = cloneStringCounts(t.gasChangeReasons)
		txCopy.GasChangeDebitByReason = cloneStringCounts(t.gasChangeDebitByReason)
		txCopy.GasChangeCreditByReason = cloneStringCounts(t.gasChangeCreditByReason)
		txCopy.GasChangeNetByReason = cloneGasNet(t.gasChangeDebitByReason, t.gasChangeCreditByReason)
		txCopy.BlockHashReads = cloneBlockHashReads(t.blockHashReads)
		result.Tx = &txCopy
	}
	if t.config.IncludeCallFrames {
		result.Frames = make([]pgoCallFrameSummary, 0, len(selectedFrames))
		for _, frame := range selectedFrames {
			result.Frames = append(result.Frames, frame.summary)
		}
	}
	if t.config.IncludeProfiles {
		result.Profiles = make([]pgoCodeProfile, 0, len(selectedFrames))
		for _, frame := range selectedFrames {
			result.Profiles = append(result.Profiles, frame.profile)
		}
	}
	res, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return res, t.reason
}

func (t *pgoTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}
