#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GETH_BIN="${ROOT_DIR}/build/bin/geth"

DATADIR="${DATADIR:-${SCRIPT_DIR}/data/geth-300k}"
HTTP_ADDR="${HTTP_ADDR:-127.0.0.1}"
HTTP_PORT="${HTTP_PORT:-8545}"
IPC_PATH="${IPC_PATH:-${DATADIR}/geth.ipc}"
CACHE_MB="${CACHE_MB:-2048}"

if [[ ! -x "${GETH_BIN}" ]]; then
  echo "geth binary not found at ${GETH_BIN}" >&2
  exit 1
fi

if [[ ! -d "${DATADIR}" ]]; then
  echo "datadir not found: ${DATADIR}" >&2
  exit 1
fi

echo "Starting geth in offline analysis mode."
echo "datadir : ${DATADIR}"
echo "ipc     : ${IPC_PATH}"
echo

exec "${GETH_BIN}" \
  --mainnet \
  --datadir "${DATADIR}" \
  --syncmode full \
  --gcmode full \
  --state.scheme hash \
  --nodiscover \
  --maxpeers 0 \
  --ipcpath "${IPC_PATH}" \
  --http \
  --http.addr "${HTTP_ADDR}" \
  --http.port "${HTTP_PORT}" \
  --http.api eth,debug \
  --cache "${CACHE_MB}"
