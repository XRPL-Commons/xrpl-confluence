"""Shrink test suite.

Single-probe shrinker: replays a run-log prefix `[0..shrink_max_step]` against
the topology and reports whether the original divergence reproduced. The
external bisect driver (`scripts/shrink.sh`) calls this suite with varying
`shrink_max_step` to find the minimal failing prefix.

The driver pre-uploads the run log + divergence JSON as a Kurtosis files
artifact named `shrink_artifact` (containing `run.ndjson` and `div.json`).
"""

helpers = import_module("../helpers/rpc.star")
fuzz = import_module("../sidecar/fuzz.star")


def run(
        plan,
        nodes,
        shrink_artifact,
        shrink_max_step,
        image = "xrpl-confluence-sidecar:latest",
        accounts = 10,
        seed = None,
        validate_timeout = "60s"):
    """Run the shrink probe.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.
        shrink_artifact: Name of a Kurtosis files artifact containing
            `run.ndjson` and `div.json` at its root.
        shrink_max_step: Inclusive prefix cap on RunLogEntry.Step.
        image: Docker image for the shrink sidecar.
        accounts: Account pool size (must match the original run).
        seed: uint64 seed (must match the original run for deterministic
            account derivation).
        validate_timeout: Per-tx wait for `validated:true` on every node.

    Returns:
        Results dict.
    """
    plan.print("Waiting for all nodes to reach closed_seq >= 3...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    plan.print("Launching shrink sidecar (max_step={})".format(shrink_max_step))
    fuzz.launch(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        image = image,
        mode = "shrink",
        accounts = accounts,
        seed = seed,
        shrink_artifact = shrink_artifact,
        shrink_max_step = shrink_max_step,
        shrink_validate_timeout = validate_timeout,
    )

    plan.print("Waiting for shrink completion (timeout 900s)...")
    plan.wait(
        service_name = "fuzz",
        recipe = GetHttpRequestRecipe(
            port_id = "results",
            endpoint = "/status",
            extract = {"state": ".state"},
        ),
        field = "extract.state",
        assertion = "==",
        target_value = "completed",
        timeout = "900s",
        interval = "5s",
    )

    plan.print("=== Shrink result ===")
    plan.request(
        service_name = "fuzz",
        recipe = GetHttpRequestRecipe(
            port_id = "results",
            endpoint = "/status",
        ),
    )

    return {"shrink": "completed"}
