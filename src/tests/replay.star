"""Replay suite: fetch mainnet ledgers and replay txs against the topology."""

helpers = import_module("../helpers/rpc.star")
replay = import_module("../sidecar/replay.star")


def run(
    plan,
    nodes,
    image = "trafficgen:latest",
    mainnet_url = "https://s1.ripple.com:51234",
    ledger_start = 80000000,
    ledger_end = 80000005,
    accounts = 10,
    seed = None,
):
    """Run the replay suite.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.
        image: Docker image for the replay sidecar.
        mainnet_url: Public rippled JSON-RPC endpoint to fetch ledgers from.
        ledger_start: First mainnet ledger index to replay (inclusive).
        ledger_end: Last mainnet ledger index to replay (inclusive).
        accounts: Account pool size.
        seed: Optional uint64 fuzz seed for reproducibility.

    Returns:
        Results dict.
    """
    plan.print("Waiting for all nodes to reach closed_seq >= 3...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    plan.print("Launching replay sidecar (range {}..{})".format(ledger_start, ledger_end))
    replay.launch(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        image = image,
        mainnet_url = mainnet_url,
        ledger_start = ledger_start,
        ledger_end = ledger_end,
        accounts = accounts,
        seed = seed,
    )

    plan.print("Waiting for replay completion (timeout 900s)...")
    plan.wait(
        service_name = "replay",
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

    plan.print("=== Replay results ===")
    plan.request(
        service_name = "replay",
        recipe = GetHttpRequestRecipe(
            port_id = "results",
            endpoint = "/status",
        ),
    )

    return {"replay": "completed"}
