#!/usr/bin/env python3
"""Non-interactive remote pair helper.

Replaces `pymobiledevice3 remote pair` which otherwise opens an interactive
menu to pick an IP (problematic when stdout is piped). We browse via Bonjour,
filter by device name, prefer the IPv4 address, and autopair.
"""
from __future__ import annotations
import asyncio
import builtins
import sys

# Flush interactive prompts on their own line so the parent (Go subprocess with
# line-based scanner) can render the prompt in real time.
_orig_input = builtins.input
def _line_input(prompt: str = "") -> str:
    if prompt:
        sys.stdout.write(f"{prompt}\n")
        sys.stdout.flush()
    return _orig_input()
builtins.input = _line_input

from pymobiledevice3.cli.remote import browse_remotepairing_manual_pairing
from pymobiledevice3.exceptions import RemotePairingCompletedError
from pymobiledevice3.remote.tunnel_service import RemotePairingManualPairingService


def _score(ip: str) -> int:
    # Prefer IPv4 addresses, then IPv6 non-link-local, then link-local.
    if "." in ip and ":" not in ip:
        return 0
    if ip.startswith("fe80") or "%" in ip:
        return 2
    return 1


async def main(name: str) -> int:
    print(f"browsing for {name or '<any>'} (up to 10s)…", flush=True)
    # Retry with increasing timeout — the first mDNS browse after a process
    # starts often misses early announcements.
    answers = []
    for timeout in (3, 5, 10):
        answers = await browse_remotepairing_manual_pairing(timeout=timeout)
        if answers:
            print(f"found {len(answers)} advertisement(s) with timeout={timeout}s", flush=True)
            break
        print(f"  no results within {timeout}s, retrying…", flush=True)

    if not answers:
        print("ERROR: no devices advertised _remotepairing._tcp", flush=True)
        print("If you see Go-side discovery working but Python not, grant Local Network permission:", flush=True)
        print("  System Settings → Privacy & Security → Local Network → iNoaload: ON", flush=True)
        return 2

    candidates = []
    for answer in answers:
        current = answer.properties.get("name", "")
        if name and current != name:
            continue
        for addr in answer.addresses:
            candidates.append(
                (
                    answer.properties["identifier"],
                    addr.full_ip,
                    answer.port,
                    current,
                )
            )

    if not candidates:
        print(f"ERROR: device '{name}' not found", flush=True)
        return 3

    # Deduplicate by (identifier, ip) and pick the best address.
    seen = set()
    unique = []
    for ident, ip, port, dn in candidates:
        key = (ident, ip)
        if key in seen:
            continue
        seen.add(key)
        unique.append((ident, ip, port, dn))
    unique.sort(key=lambda c: _score(c[1]))

    ident, ip, port, dn = unique[0]
    print(f"pairing with {dn} at {ip}:{port}", flush=True)
    print("accept the trust prompt on the device now…", flush=True)

    try:
        async with RemotePairingManualPairingService(ident, ip, port) as svc:
            await svc.connect(autopair=True)
    except RemotePairingCompletedError:
        # Not an error — the device closes the pairing session right after
        # success. Record was written to ~/.pymobiledevice3/.
        pass

    # Go side persists the paired state in the app's SQLite DB when it sees
    # this SUCCESS line on the WebSocket stream.
    print("SUCCESS: paired and pair record saved", flush=True)
    return 0


if __name__ == "__main__":
    name = sys.argv[1] if len(sys.argv) > 1 else ""
    try:
        sys.exit(asyncio.run(main(name)))
    except Exception as exc:
        print(f"ERROR: {type(exc).__name__}: {exc}", flush=True)
        sys.exit(1)
