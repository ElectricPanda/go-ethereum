#!/usr/bin/env python3
"""Fetch block metadata from Etherscan v2 proxy API."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request


def fetch_block(block_number: int, api_key: str) -> dict:
    params = {
        "chainid": "1",
        "module": "proxy",
        "action": "eth_getBlockByNumber",
        "tag": hex(block_number),
        "boolean": "false",
        "apikey": api_key,
    }
    url = "https://api.etherscan.io/v2/api?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={"accept": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        body = json.load(resp)
    if "result" not in body or body["result"] is None:
        raise RuntimeError(f"unexpected Etherscan response: {body}")
    return body["result"]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--block", type=int, required=True, help="Target block number")
    parser.add_argument(
        "--api-key",
        default=os.environ.get("ETHERSCAN_API_KEY", ""),
        help="Etherscan API key (default: ETHERSCAN_API_KEY env var)",
    )
    args = parser.parse_args()

    if not args.api_key:
        print("missing API key: set ETHERSCAN_API_KEY or pass --api-key", file=sys.stderr)
        return 2

    try:
        block = fetch_block(args.block, args.api_key)
    except urllib.error.URLError as exc:
        print(f"failed to query Etherscan: {exc}", file=sys.stderr)
        return 1
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    out = {
        "number": int(block["number"], 16),
        "hash": block["hash"],
        "parentHash": block["parentHash"],
        "stateRoot": block["stateRoot"],
        "timestamp": int(block["timestamp"], 16),
    }
    print(json.dumps(out, indent=2))
    print()
    print(f"TARGET_HASH={out['hash']}")
    print(f"TARGET_BLOCK={out['number']}")
    print(f"TARGET_STATE_ROOT={out['stateRoot']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
