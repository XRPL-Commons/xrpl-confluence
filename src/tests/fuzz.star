"""Fuzz test suite.

Launches the fuzz sidecar against the mixed network and waits for it to
submit `tx_count` transactions. Divergences are written into the sidecar's
corpus directory; the suite reports submitted/succeeded/failed counts.
"""

helpers = import_module("../helpers/rpc.star")
fuzz = import_module("../sidecar/fuzz.star")


def run(plan, nodes, image = "trafficgen:latest", tx_count = 100, accounts = 10, seed = None):
    """Run the fuzz suite.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.
        image: Docker image for the fuzz sidecar.
        tx_count: Total transactions to submit.
        accounts: Account pool size.
        seed: Optional uint64 fuzz seed for reproducibility.

    Returns:
        Results dict.
    """
    # Wait for the network to be live.
    plan.print("Waiting for all nodes to reach closed_seq >= 3...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    plan.print("Launching fuzz sidecar (tx_count={}, accounts={}, seed={})...".format(
        tx_count, accounts, seed if seed != None else "random"))
    fuzz.launch(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        image = image,
        tx_count = tx_count,
        accounts = accounts,
        seed = seed,
    )

    plan.print("Waiting for fuzz to complete (timeout 600s)...")
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
        timeout = "600s",
        interval = "5s",
    )

    plan.print("=== Fuzz results ===")
    plan.request(
        service_name = "fuzz",
        recipe = GetHttpRequestRecipe(
            port_id = "results",
            endpoint = "/status",
        ),
    )

    return {"fuzz": "completed"}
