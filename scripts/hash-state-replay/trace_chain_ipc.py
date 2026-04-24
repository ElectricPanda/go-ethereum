#!/usr/bin/env python3
"""Call debug_traceChain over Geth IPC using only the Python standard library."""

from __future__ import annotations

import argparse
import json
import socket
import sys
import time
from typing import Any


class IPCClient:
    def __init__(self, path: str) -> None:
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(path)
        self.sock.settimeout(60)
        self.decoder = json.JSONDecoder()
        self.buffer = ""

    def close(self) -> None:
        try:
            self.sock.close()
        except OSError:
            pass

    def _recv_one(self) -> dict[str, Any]:
        while True:
            stripped = self.buffer.lstrip()
            if stripped:
                offset = len(self.buffer) - len(stripped)
                try:
                    obj, end = self.decoder.raw_decode(stripped)
                except json.JSONDecodeError:
                    pass
                else:
                    self.buffer = stripped[end:]
                    return obj
                if offset:
                    self.buffer = stripped

            chunk = self.sock.recv(65536)
            if not chunk:
                raise RuntimeError("IPC connection closed")
            self.buffer += chunk.decode("utf-8")

    def call(self, method: str, params: list[Any], request_id: int = 1) -> Any:
        payload = {
            "jsonrpc": "2.0",
            "id": request_id,
            "method": method,
            "params": params,
        }
        self.sock.sendall(json.dumps(payload).encode("utf-8"))
        while True:
            msg = self._recv_one()
            if msg.get("id") != request_id:
                continue
            if "error" in msg:
                raise RuntimeError(msg["error"])
            return msg["result"]

    def next_notification(self, method: str, subscription_id: str) -> dict[str, Any]:
        while True:
            msg = self._recv_one()
            if msg.get("method") != method:
                continue
            params = msg.get("params", {})
            if params.get("subscription") != subscription_id:
                continue
            return params["result"]

    def unsubscribe(self, namespace: str, subscription_id: str, request_id: int = 2) -> Any:
        return self.call(f"{namespace}_unsubscribe", [subscription_id], request_id=request_id)


def parse_block_number(value: Any) -> int:
    if isinstance(value, int):
        return value
    if isinstance(value, str):
        if value.startswith(("0x", "0X")):
            return int(value, 16)
        return int(value)
    raise TypeError(f"unsupported block number type: {type(value)!r}")


def tx_count_of_block(client: IPCClient, block_number: int) -> int:
    block = client.call("eth_getBlockByNumber", [hex(block_number), False], request_id=10_000 + block_number)
    if block is None:
        raise RuntimeError(f"block {block_number} not found")
    txs = block.get("transactions", [])
    return len(txs)


def trace_block_by_number(client: IPCClient, block_number: int, tracer: str, reexec: int) -> list[Any]:
    result = client.call(
        "debug_traceBlockByNumber",
        [
            hex(block_number),
            {
                "reexec": reexec,
                "tracer": tracer,
            },
        ],
        request_id=20_000 + block_number,
    )
    if not isinstance(result, list):
        raise RuntimeError(f"unexpected traceBlockByNumber result type: {type(result)!r}")
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ipc", required=True, help="Path to geth IPC socket")
    parser.add_argument("--start", type=int, required=True, help="First block to replay")
    parser.add_argument("--end", type=int, required=True, help="Included end block")
    parser.add_argument("--tracer", default="callTracer", help="Tracer name")
    parser.add_argument("--reexec", type=int, default=0, help="Reexec budget")
    parser.add_argument("--out", default="", help="Optional JSONL output file")
    parser.add_argument("--skip-preflight", action="store_true", help="Skip traceBlockByNumber preflight on the first block")
    args = parser.parse_args()

    if args.start <= 0:
        print("start must be greater than 0", file=sys.stderr)
        return 2
    if args.start > args.end:
        print("start must be smaller than or equal to end", file=sys.stderr)
        return 2

    client = IPCClient(args.ipc)
    out_fh = open(args.out, "w", encoding="utf-8") if args.out else None
    sub_id = ""
    try:
        if not args.skip_preflight:
            traces = trace_block_by_number(client, args.start, args.tracer, args.reexec)
            print(f"preflight ok block={args.start} traces={len(traces)}")

        expected_blocks = []
        for block_number in range(args.start, args.end + 1):
            tx_count = tx_count_of_block(client, block_number)
            if tx_count > 0 or block_number == args.end:
                expected_blocks.append((block_number, tx_count))

        if not expected_blocks:
            print("traceChain has no expected notifications for this range")
            return 0

        rpc_start = args.start - 1
        sub_id = client.call(
            "debug_subscribe",
            [
                "traceChain",
                hex(rpc_start),
                hex(args.end),
                {
                    "reexec": args.reexec,
                    "tracer": args.tracer,
                },
            ],
        )
        print(f"subscription={sub_id}")

        for expected, tx_count in expected_blocks:
            result = client.next_notification("debug_subscription", sub_id)
            block_no = parse_block_number(result["block"])
            if block_no != expected:
                raise RuntimeError(f"unexpected block {block_no}, expected {expected}")

            traces = result["traces"]
            print(f"block={block_no} txs={len(traces)}")
            if tx_count > 0 and len(traces) != tx_count:
                raise RuntimeError(f"unexpected trace count for block {block_no}: have {len(traces)}, expected {tx_count}")

            if out_fh is not None:
                out_fh.write(json.dumps(result))
                out_fh.write("\n")
                out_fh.flush()

        print("traceChain succeeded")
        return 0
    except (OSError, RuntimeError, TimeoutError) as exc:
        print(f"trace failed: {exc}", file=sys.stderr)
        return 1
    finally:
        if sub_id:
            try:
                client.unsubscribe("debug", sub_id)
            except Exception:
                pass
        if out_fh is not None:
            out_fh.close()
        client.close()
        # Give the server a brief moment before the socket disappears.
        time.sleep(0.1)


if __name__ == "__main__":
    raise SystemExit(main())
