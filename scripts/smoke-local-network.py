#!/usr/bin/env python3
"""Exercise custom images, consensus, payments, resume, and reset against Docker."""

import argparse
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid


def command(args, cwd=None, timeout=600):
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"{' '.join(args[:4])} failed:\n{result.stdout}\n{result.stderr}")
    return result.stdout


def rpc(url, method, params):
    request = urllib.request.Request(
        url,
        json.dumps({"method": method, "params": [params]}).encode(),
        {"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.load(response)["result"]


def balance(nodes, expected):
    for node in nodes:
        result = rpc(node["rpc"], "account_info", {
            "account": "ra8sezk7XT7JRgE1myhUBZJUDCUH3qrWMU", "ledger_index": "validated",
        })
        if expected is None:
            assert result.get("error") == "actNotFound", result
        else:
            assert result.get("account_data", {}).get("Balance") == expected, result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="Existing local rippled image; never modified")
    parser.add_argument("--entrypoint", required=True, help="Absolute rippled executable path inside the image")
    parser.add_argument("--sidecar-image", help="Also test dashboard/control on reset using this built image")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    suffix = uuid.uuid4().hex[:10]
    enclave = f"confluence-smoke-{suffix}"
    image = f"confluence-smoke:{suffix}"
    platform = command(["docker", "image", "inspect", args.image, "--format", "{{.Os}}/{{.Architecture}}"]).strip()
    with tempfile.TemporaryDirectory(prefix="confluence-smoke-") as directory:
        work = Path(directory)
        binary = work / "confluence"
        command(["go", "build", "-o", str(binary), "./cmd/confluence"], cwd=root / "sidecar")
        scenario = {
            "apiVersion": "confluence/v1", "kind": "Scenario", "metadata": {"name": enclave},
            "topology": {
                "rippled": {
                    "count": 2, "image": image, "entrypoint": [args.entrypoint],
                    "nodes": [{}, {"image": args.image, "config": {"peers_max": ["23"]}}],
                },
                "goxrpl": {"count": 0},
            },
            "network": {"network_id": 10000, "veto_amendments": [{
                "id": "56B241D7A43D40354D02A9DC4C8DF5C7A1F930D92A9035C4E12291B3CA3E1C2B", "name": "Clawback",
            }]},
            "workload": {"kind": "none"},
            "services": {"dashboard": False, "control": False},
        }
        scenario_path = work / "network.yaml"
        scenario_path.write_text(json.dumps(scenario))

        def build(revision):
            (work / "Dockerfile").write_text(f"FROM {args.image}\nLABEL confluence.smoke.revision={revision}\n")
            command(["docker", "build", "--platform", platform, "-t", image, str(work)])

        launched = False

        def up(*flags):
            nonlocal launched
            launched = True
            output = command([
                str(binary), "up", "-f", str(scenario_path), "--package", str(root), "--json", *flags,
            ], cwd=work)
            return json.loads(output)

        try:
            build("one")
            before_image = command(["docker", "image", "inspect", image, "--format", "{{.Id}}"])
            current = up()
            nodes = current["nodes"]
            assert len(nodes) == 2 and not current.get("control_url") and not current.get("dashboard_url"), current
            print("PASS: two custom-image validators ready without dashboard, control, or fuzzer", flush=True)
            balance(nodes, None)
            result = rpc(nodes[0]["rpc"], "submit", {
                "secret": "snoPBrXtMeMyMHUVTgbuqAfg1SUTb",
                "tx_json": {
                    "TransactionType": "Payment", "Account": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
                    "Destination": "ra8sezk7XT7JRgE1myhUBZJUDCUH3qrWMU", "Amount": "100000000", "NetworkID": 10000,
                },
            })
            assert result.get("engine_result") == "tesSUCCESS", result
            tx_hash = result["tx_json"]["hash"]
            deadline = time.monotonic() + 90
            while True:
                transactions = [rpc(node["rpc"], "tx", {"transaction": tx_hash}) for node in nodes]
                if all(tx.get("validated") and tx.get("meta", {}).get("TransactionResult") == "tesSUCCESS" for tx in transactions):
                    break
                if time.monotonic() >= deadline:
                    raise RuntimeError(f"payment did not validate: {transactions}")
                time.sleep(1)
            indexes = [tx["ledger_index"] for tx in transactions]
            assert len(set(indexes)) == 1, indexes
            hashes = [rpc(node["rpc"], "ledger", {"ledger_index": indexes[0]})["ledger_hash"] for node in nodes]
            assert len(set(hashes)) == 1, hashes
            balance(nodes, "100000000")
            print(f"PASS: payment {tx_hash} validated on both nodes in ledger {indexes[0]}", flush=True)

            result = subprocess.run([str(binary), "up", "-f", str(scenario_path), "--package", str(root)], cwd=work, text=True, capture_output=True, timeout=60)
            assert result.returncode and "--resume" in result.stderr, result
            build("two")
            after_image = command(["docker", "image", "inspect", image, "--format", "{{.Id}}"])
            assert before_image != after_image, "rebuild must change image contents"
            current = up("--resume")
            balance(current["nodes"], "100000000")
            containers = command(["docker", "ps", "--filter", f"ancestor={image}", "--format", "{{.ID}} {{.Image}}"])
            assert image in containers, containers
            print("PASS: same-tag image rebuild/resume retained the funded balance", flush=True)
            endpoints = json.loads(command([str(binary), "endpoints", "--json"], cwd=work))
            assert endpoints["nodes"] == current["nodes"], endpoints

            if args.sidecar_image:
                scenario["services"] = {"dashboard": True, "control": True, "sidecar_image": args.sidecar_image}
                scenario_path.write_text(json.dumps(scenario))
            current = up("--reset")
            balance(current["nodes"], None)
            if args.sidecar_image:
                for url in [current["dashboard_url"], current["control_url"] + "/v1/healthz"]:
                    with urllib.request.urlopen(url, timeout=10) as response:
                        assert response.status == 200
            print("PASS: explicit reset created fresh ledger state; requested services are reachable", flush=True)
            command([str(binary), "down"], cwd=work)
            launched = False
            print("PASS: down removed the enclave and ledger data", flush=True)
        finally:
            if launched:
                cleanup = subprocess.run(["kurtosis", "enclave", "rm", "-f", enclave], capture_output=True, text=True, timeout=60)
                if cleanup.returncode:
                    if sys.exc_info()[0] is None:
                        raise RuntimeError(f"cleanup failed for {enclave}: {cleanup.stderr}")
                    print(f"Cleanup failed for {enclave}: {cleanup.stderr}", file=sys.stderr)
            subprocess.run(["docker", "image", "rm", image], capture_output=True, text=True, timeout=60)


if __name__ == "__main__":
    main()
