import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).parents[1] / "src/network/readiness.py"
SPEC = importlib.util.spec_from_file_location("network_readiness", MODULE_PATH)
readiness = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(readiness)


class FakeRPC:
    def __init__(self, sequences, hashes=None, errors=None):
        self.sequences = {url: list(values) for url, values in sequences.items()}
        self.hashes = hashes or {}
        self.errors = errors or {}
        self.calls = []
        self.info_calls = {url: 0 for url in self.sequences}

    def __call__(self, url, method, params):
        self.calls.append((url, method, dict(params)))
        error = self.errors.get((url, method))
        if error is not None:
            raise readiness.ReadinessError(error)
        if method == "server_info":
            index = self.info_calls[url]
            self.info_calls[url] += 1
            values = self.sequences[url]
            sequence = values[min(index, len(values) - 1)]
            return {
                "info": {
                    "validated_ledger": {
                        "seq": sequence,
                        "hash": "latest-{}".format(sequence),
                    }
                }
            }
        if method == "ledger":
            index = params["ledger_index"]
            return {"ledger": {"ledger_hash": self.hashes.get((url, index), "COMMON")}}
        raise AssertionError(method)


class NetworkReadinessTest(unittest.TestCase):
    def test_structured_rpc_error_is_reported(self):
        class Response:
            def read(self):
                return b'{"result":{"status":"error","error":"nope","error_message":"ledger unavailable"}}'

            def close(self):
                pass

        with self.assertRaisesRegex(readiness.ReadinessError, r"RPC server_info returned status 'error'"):
            readiness._rpc_call("http://n0", "server_info", opener=lambda request, timeout: Response())

    def test_waits_for_advance_then_queries_one_fixed_index(self):
        rpc = FakeRPC(
            {"http://n0": [1, 2], "http://n1": [1, 3]},
            {("http://n0", 2): "abc", ("http://n1", 2): "ABC"},
        )
        now = [0.0]
        result = readiness.wait_for_common_validated_ledger(
            rpc.sequences,
            timeout_seconds=10,
            poll_interval_seconds=1,
            request=rpc,
            clock=lambda: now[0],
            sleep=lambda seconds: now.__setitem__(0, now[0] + seconds),
        )
        self.assertEqual(result["ledger_index"], 2)
        self.assertEqual(result["ledger_hash"], "ABC")
        ledger_calls = [call for call in rpc.calls if call[1] == "ledger"]
        self.assertTrue(ledger_calls)
        self.assertEqual({call[2]["ledger_index"] for call in ledger_calls}, {2})
        self.assertFalse(any(call[1] == "ledger" and call[2].get("ledger_index") == "latest" for call in rpc.calls))

    def test_stalled_node_times_out_with_per_node_diagnostics(self):
        rpc = FakeRPC({"http://n0": [1, 2], "http://n1": [1]})
        now = [0.0]
        with self.assertRaisesRegex(readiness.ReadinessError, r"node-1: seq=1 baseline=1"):
            readiness.wait_for_common_validated_ledger(
                rpc.sequences,
                timeout_seconds=2,
                poll_interval_seconds=1,
                request=rpc,
                clock=lambda: now[0],
                sleep=lambda seconds: now.__setitem__(0, now[0] + seconds),
            )

    def test_mismatched_hashes_are_not_accepted(self):
        rpc = FakeRPC(
            {"http://n0": [1, 2], "http://n1": [1, 2]},
            {("http://n0", 2): "AAA", ("http://n1", 2): "BBB"},
        )
        now = [0.0]
        with self.assertRaisesRegex(readiness.ReadinessError, r"hash mismatch"):
            readiness.wait_for_common_validated_ledger(
                rpc.sequences,
                timeout_seconds=2,
                poll_interval_seconds=1,
                request=rpc,
                clock=lambda: now[0],
                sleep=lambda seconds: now.__setitem__(0, now[0] + seconds),
            )

    def test_rpc_error_is_reported(self):
        rpc = FakeRPC(
            {"http://n0": [1], "http://n1": [1]},
            errors={("http://n1", "server_info"): "connection refused"},
        )
        now = [0.0]
        with self.assertRaisesRegex(readiness.ReadinessError, r"connection refused"):
            readiness.wait_for_common_validated_ledger(
                rpc.sequences,
                timeout_seconds=2,
                poll_interval_seconds=1,
                request=rpc,
                clock=lambda: now[0],
                sleep=lambda seconds: now.__setitem__(0, now[0] + seconds),
            )


if __name__ == "__main__":
    unittest.main()
