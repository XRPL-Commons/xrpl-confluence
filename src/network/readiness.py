"""Validated-ledger readiness checks for local rippled networks.

The script is deliberately stdlib-only because it is copied into a short-lived
``python:3.11-alpine`` Kurtosis task.  A node is ready only after its validated
ledger advances past the first observed value and every node agrees on one
fixed ledger index and hash.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from typing import Any, Callable, Dict, Iterable, List, Mapping, Optional, Sequence


DEFAULT_TIMEOUT_SECONDS = 180.0
DEFAULT_POLL_INTERVAL_SECONDS = 2.0
RPC_TIMEOUT_SECONDS = 5.0


class ReadinessError(RuntimeError):
    """Raised when the network cannot prove common validated-ledger progress."""


def _rpc_call(
    url: str,
    method: str,
    params: Optional[Mapping[str, Any]] = None,
    timeout: float = RPC_TIMEOUT_SECONDS,
    opener: Optional[Callable[..., Any]] = None,
) -> Mapping[str, Any]:
    """Call one JSON-RPC method and return its ``result`` object.

    ``opener`` is injectable for unit tests.  The production path uses
    ``urllib.request.urlopen`` and therefore has no third-party dependency.
    """

    payload = json.dumps(
        {"method": method, "params": [dict(params or {})]},
        separators=(",", ":"),
    ).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    open_request = opener or urllib.request.urlopen
    try:
        response = open_request(request, timeout=timeout)
        try:
            body = response.read()
        finally:
            close = getattr(response, "close", None)
            if close is not None:
                close()
    except (OSError, urllib.error.URLError, TimeoutError) as exc:
        raise ReadinessError("RPC {} failed: {}".format(method, exc)) from exc

    try:
        document = json.loads(body.decode("utf-8"))
    except (AttributeError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ReadinessError("RPC {} returned invalid JSON: {}".format(method, exc)) from exc

    if not isinstance(document, Mapping):
        raise ReadinessError("RPC {} returned a non-object response".format(method))
    if document.get("error") is not None:
        error = document["error"]
        if isinstance(error, Mapping):
            detail = error.get("message") or error.get("error") or repr(error)
        else:
            detail = repr(error)
        raise ReadinessError("RPC {} returned error: {}".format(method, detail))
    result = document.get("result")
    if not isinstance(result, Mapping):
        raise ReadinessError("RPC {} response has no result object".format(method))
    status = result.get("status")
    if status not in (None, "success"):
        raise ReadinessError("RPC {} returned status {!r}".format(method, status))
    return result


def _validated_ledger(info_result: Mapping[str, Any]) -> tuple[int, str]:
    """Extract and validate the ``server_info`` validated-ledger tuple."""

    info = info_result.get("info", info_result)
    if not isinstance(info, Mapping):
        raise ReadinessError("server_info result has no info object")
    ledger = info.get("validated_ledger")
    if not isinstance(ledger, Mapping):
        raise ReadinessError("server_info has no validated_ledger")
    sequence = ledger.get("seq")
    ledger_hash = ledger.get("hash")
    if isinstance(sequence, bool):
        raise ReadinessError("validated ledger sequence is boolean")
    try:
        sequence = int(sequence)
    except (TypeError, ValueError) as exc:
        raise ReadinessError("validated ledger sequence is invalid: {!r}".format(sequence)) from exc
    if sequence < 1 or not isinstance(ledger_hash, str) or not ledger_hash:
        raise ReadinessError(
            "validated ledger is incomplete: seq={!r} hash={!r}".format(sequence, ledger_hash)
        )
    return sequence, ledger_hash.upper()


def _ledger_hash(ledger_result: Mapping[str, Any]) -> str:
    ledger = ledger_result.get("ledger")
    if not isinstance(ledger, Mapping):
        raise ReadinessError("ledger response has no ledger object")
    # ``server_info`` calls this field ``hash`` while the ``ledger`` method
    # returns ``ledger_hash``.  Keep the fallback for lightweight RPC fakes and
    # alternate implementations, but prefer rippled's wire field.
    ledger_hash = ledger.get("ledger_hash", ledger.get("hash"))
    if not isinstance(ledger_hash, str) or not ledger_hash:
        raise ReadinessError("ledger response has no hash")
    return ledger_hash.upper()


def _diagnostic_lines(states: Sequence[Mapping[str, Any]]) -> List[str]:
    lines: List[str] = []
    for state in states:
        name = state["name"]
        sequence = state.get("seq")
        baseline = state.get("baseline")
        target = state.get("target")
        ledger_hash = state.get("hash")
        error = state.get("error")
        if error:
            detail = "error={}".format(error)
        else:
            detail = "seq={} baseline={} target={} hash={}".format(
                sequence,
                baseline,
                target,
                ledger_hash or "<pending>",
            )
        lines.append("{}: {}".format(name, detail))
    return lines


def wait_for_common_validated_ledger(
    urls: Iterable[str],
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS,
    poll_interval_seconds: float = DEFAULT_POLL_INTERVAL_SECONDS,
    request: Optional[Callable[..., Mapping[str, Any]]] = None,
    clock: Callable[[], float] = time.monotonic,
    sleep: Callable[[float], None] = time.sleep,
) -> Dict[str, Any]:
    """Wait for all URLs to advance and agree on one fixed ledger.

    The target index is selected once, after every node has advanced beyond its
    baseline.  Subsequent polls query that same index, which avoids falsely
    accepting a network merely because each node reports a different moving
    ``latest`` hash.
    """

    node_urls = [str(url) for url in urls]
    if not node_urls:
        raise ReadinessError("no node RPC URLs were supplied")
    if timeout_seconds <= 0:
        raise ReadinessError("timeout_seconds must be positive")
    if poll_interval_seconds < 0:
        raise ReadinessError("poll_interval_seconds cannot be negative")

    states: List[Dict[str, Any]] = [
        {"name": "node-{}".format(index), "url": url, "baseline": None, "seq": None}
        for index, url in enumerate(node_urls)
    ]
    deadline = clock() + float(timeout_seconds)
    target: Optional[int] = None
    target_hashes: Dict[str, str] = {}

    def call_rpc(url: str, method: str, params: Mapping[str, Any]) -> Mapping[str, Any]:
        """Call an injected fake or cap production RPC timeouts at the deadline."""
        if request is not None:
            return request(url, method, params)
        remaining = deadline - clock()
        if remaining <= 0:
            raise ReadinessError("readiness deadline reached before RPC {}".format(method))
        return _rpc_call(url, method, params, timeout=min(RPC_TIMEOUT_SECONDS, remaining))

    while True:
        now = clock()
        if now >= deadline:
            details = "\n".join(_diagnostic_lines(states))
            raise ReadinessError(
                "timed out after {:.1f}s waiting for all nodes to advance and agree on "
                "one validated ledger\n{}".format(timeout_seconds, details)
            )

        for state in states:
            state.pop("error", None)
            state.pop("hash", None)
            try:
                server_info = call_rpc(state["url"], "server_info", {})
                sequence, ledger_hash = _validated_ledger(server_info)
                state["seq"] = sequence
                state["latest_hash"] = ledger_hash
                if state["baseline"] is None:
                    state["baseline"] = sequence
            except ReadinessError as exc:
                state["error"] = str(exc)

        if target is None and all(
            state.get("baseline") is not None
            and state.get("seq") is not None
            and state["seq"] > state["baseline"]
            for state in states
        ):
            target = min(state["seq"] for state in states)
            for state in states:
                state["target"] = target

        if target is not None:
            target_hashes = {}
            for state in states:
                if state.get("error") or state.get("seq", 0) < target:
                    continue
                try:
                    ledger = call_rpc(
                        state["url"],
                        "ledger",
                        {"ledger_index": target, "transactions": False, "expand": False},
                    )
                    target_hashes[state["name"]] = _ledger_hash(ledger)
                    state["hash"] = target_hashes[state["name"]]
                except ReadinessError as exc:
                    state["error"] = str(exc)

            if len(target_hashes) == len(states):
                hashes = set(target_hashes.values())
                if len(hashes) == 1:
                    agreed_hash = next(iter(hashes))
                    return {
                        "ledger_index": target,
                        "ledger_hash": agreed_hash,
                        "nodes": [
                            {
                                "name": state["name"],
                                "url": state["url"],
                                "validated_seq": state["seq"],
                                "ledger_hash": state.get("hash"),
                            }
                            for state in states
                        ],
                    }
                # Preserve the mismatch in diagnostics while allowing a brief
                # convergence window before the bounded deadline expires.
                for state in states:
                    state["error"] = "ledger {} hash mismatch: {}".format(
                        target,
                        state.get("hash", "<missing>"),
                    )

        remaining = deadline - clock()
        if remaining <= 0:
            continue
        sleep(min(float(poll_interval_seconds), remaining))


def _parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--timeout-seconds", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--poll-interval-seconds", type=float, default=DEFAULT_POLL_INTERVAL_SECONDS)
    parser.add_argument("urls", nargs="+", help="internal HTTP RPC URLs")
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    options = _parse_args(argv if argv is not None else sys.argv[1:])
    try:
        result = wait_for_common_validated_ledger(
            options.urls,
            timeout_seconds=options.timeout_seconds,
            poll_interval_seconds=options.poll_interval_seconds,
        )
    except ReadinessError as exc:
        print("Network readiness failed: {}".format(exc), file=sys.stderr)
        return 1

    print(
        "Network ready at validated ledger {} ({})".format(
            result["ledger_index"], result["ledger_hash"]
        )
    )
    for node in result["nodes"]:
        print(
            "{}: validated_seq={} ledger_hash={}".format(
                node["name"], node["validated_seq"], node["ledger_hash"]
            )
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
