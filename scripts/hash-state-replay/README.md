# Hash State Replay Scripts

This directory contains the helper scripts and notes for the
`research/hash-state-replay` branch.

The branch keeps a sparse set of historical starting states on disk during
`hash`-scheme full sync so later block or transaction replay can begin from a
persisted checkpoint instead of requiring a full archive node.

## Checkpoint Policy

The checkpoint logic lives in `core/blockchain.go` inside `writeBlockWithState`.

The current branch configuration assumes:

- `stateCheckpointInterval = 100_000`
- a checkpoint is committed when `currentBlock + 1` is a multiple of `100000`
- this means block `99,999` is committed so block `100,000` can later be replayed with `reexec=0`
- block-number-based forks use the same rule: commit the block immediately before the fork
- timestamp-based forks commit the parent state when the first post-fork block arrives

In practice, a replay that starts at block `100000`, `200000`, `300000`, ...
should be usable with `reexec=0` if the sync has already covered that height.

## Replay APIs

The scripts here are built around three replay-oriented APIs:

- `debug_sync(<blockHash>)`
  - starts a full sync toward a chosen target hash without requiring a consensus client during capture
- `debug_traceBlockByNumber(number, {"reexec": 0, ...})`
  - checks whether a block's starting state is already on disk
- `debug_subscribe("traceChain", startMinusOne, end, config)`
  - replays a block range sequentially from `(startMinusOne, end]`

The branch also adds `PgoTraceChain` and the native `pgoTracer` for compact
per-transaction gas, frame, diff, and code-profile data.

## Files

- `sync_to_target.sh`
  - starts geth, retries `debug_sync(<targetHash>)`, and exits when the target is reached
- `run_analysis_node.sh`
  - restarts the same datadir in offline analysis mode
- `fetch_block_hash.py`
  - optional helper to fetch target block metadata from Etherscan
- `trace_chain_ipc.py`
  - verifies checkpoint usability with a `debug_traceBlockByNumber(..., {"reexec": 0})`
    preflight, then replays a range through `debug_subscribe("traceChain", ...)`

## Typical Workflow

1. Choose a target block hash.
2. Full-sync to that hash with `sync_to_target.sh`.
3. Restart the same datadir in offline analysis mode with `run_analysis_node.sh`.
4. Preflight the first replay block with `trace_chain_ipc.py`.
5. If preflight succeeds, replay the selected block range.

## Quick Start

Fetch block info from Etherscan if you want the helper script to resolve the target hash:

```bash
export ETHERSCAN_API_KEY=...
python3 scripts/hash-state-replay/fetch_block_hash.py --block 10000000
```

Sync up to a chosen target block and exit automatically:

```bash
TARGET_BLOCK=10000000 \
TARGET_HASH=0x... \
DATADIR=/path/to/datadir \
bash scripts/hash-state-replay/sync_to_target.sh
```

If `ETHERSCAN_API_KEY` is set, `sync_to_target.sh` can fetch the target hash automatically:

```bash
export ETHERSCAN_API_KEY=...
TARGET_BLOCK=10000000 \
DATADIR=/path/to/datadir \
bash scripts/hash-state-replay/sync_to_target.sh
```

Restart the node offline for analysis:

```bash
DATADIR=/path/to/datadir \
IPC_PATH=/path/to/datadir/geth.ipc \
bash scripts/hash-state-replay/run_analysis_node.sh
```

Trace a block range that should be reachable from the checkpoint immediately before
block `9900000`:

```bash
python3 scripts/hash-state-replay/trace_chain_ipc.py \
  --ipc /path/to/datadir/geth.ipc \
  --start 9900000 \
  --end 9900020 \
  --tracer callTracer \
  --reexec 0
```

The script first calls `debug_traceBlockByNumber(start, {"reexec": 0, ...})`.
If that succeeds, it is strong evidence that the starting state is already on disk.
It then subscribes to `traceChain` to replay the selected range sequentially.
