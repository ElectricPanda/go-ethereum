#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GETH_BIN="${ROOT_DIR}/build/bin/geth"

TARGET_BLOCK="${TARGET_BLOCK:-300000}"
TARGET_HASH="${TARGET_HASH:-}"
DATADIR="${DATADIR:-${SCRIPT_DIR}/data/geth-300k}"
HTTP_ADDR="${HTTP_ADDR:-127.0.0.1}"
HTTP_PORT="${HTTP_PORT:-8545}"
CACHE_MB="${CACHE_MB:-4096}"
SYNC_RETRY_SECONDS="${SYNC_RETRY_SECONDS:-5}"

if [[ ! -x "${GETH_BIN}" ]]; then
  echo "geth binary not found at ${GETH_BIN}" >&2
  exit 1
fi

if [[ -z "${TARGET_HASH}" ]]; then
  if [[ -n "${ETHERSCAN_API_KEY:-}" ]]; then
    target_info="$(python3 "${SCRIPT_DIR}/fetch_block_hash.py" --block "${TARGET_BLOCK}")"
    printf '%s\n' "${target_info}"
    TARGET_HASH="$(printf '%s\n' "${target_info}" | awk -F= '/^TARGET_HASH=/{print $2}')"
  else
    echo "TARGET_HASH is required." >&2
    echo "Hint 1: export ETHERSCAN_API_KEY and rerun this script." >&2
    echo "Hint 2: python3 ${SCRIPT_DIR}/fetch_block_hash.py --block ${TARGET_BLOCK}" >&2
    exit 1
  fi
fi

mkdir -p "${DATADIR}"

echo "sync target block: ${TARGET_BLOCK}"
echo "sync target hash : ${TARGET_HASH}"
echo "datadir          : ${DATADIR}"
echo
echo "Starting geth first, then repeatedly calling debug_sync until the target is accepted."

"${GETH_BIN}" \
  --mainnet \
  --datadir "${DATADIR}" \
  --syncmode full \
  --gcmode full \
  --state.scheme hash \
  --exitwhensynced \
  --http \
  --http.addr "${HTTP_ADDR}" \
  --http.port "${HTTP_PORT}" \
  --http.api eth,debug \
  --cache "${CACHE_MB}" &

GETH_PID=$!
cleanup() {
  if kill -0 "${GETH_PID}" 2>/dev/null; then
    kill -INT "${GETH_PID}" 2>/dev/null || true
    wait "${GETH_PID}" || true
  fi
}
trap cleanup EXIT INT TERM

rpc_call() {
  curl -sS "http://${HTTP_ADDR}:${HTTP_PORT}" \
    -H 'content-type: application/json' \
    -d "$1"
}

echo "Waiting for RPC to come up..."
until rpc_call '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}' >/dev/null 2>&1; do
  if ! kill -0 "${GETH_PID}" 2>/dev/null; then
    wait "${GETH_PID}"
    exit 1
  fi
  sleep 1
done

echo "RPC is ready. Requesting debug_sync target..."
while true; do
  response="$(rpc_call "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"debug_sync\",\"params\":[\"${TARGET_HASH}\"]}")"
  if ! printf '%s' "${response}" | grep -q '"error"'; then
    echo "debug_sync accepted the target."
    break
  fi
  echo "debug_sync not ready yet: ${response}"
  if ! kill -0 "${GETH_PID}" 2>/dev/null; then
    wait "${GETH_PID}"
    exit 1
  fi
  sleep "${SYNC_RETRY_SECONDS}"
done

wait "${GETH_PID}"
