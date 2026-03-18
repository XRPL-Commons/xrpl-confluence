"""Delayed sync test.

Launches rippled nodes first, waits for them to advance past genesis,
then launches goXRPL to observe connection and sync behavior with a
staggered start.
"""

goxrpl = import_module("../goxrpl/goxrpl.star")

def run(plan, rippled_nodes, goxrpl_image, network_config):
    """Run the delayed sync test.

    Args:
        plan: Kurtosis plan object.
        rippled_nodes: List of already-running rippled node descriptors.
        goxrpl_image: Docker image for goXRPL.
        network_config: Shared network configuration artifact.

    Returns:
        Dict of test results.
    """
    results = {}

    # --- Step 1: Wait for rippled to advance past genesis ---
    plan.print("Waiting for rippled to reach ledger seq >= 5...")

    plan.wait(
        service_name = rippled_nodes[0]["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"seq": ".result.info.closed_ledger.seq"},
        ),
        field = "extract.seq",
        assertion = ">=",
        target_value = 5,
        timeout = "120s",
    )

    # Capture rippled state before launching goXRPL
    plan.print("Rippled advanced past genesis. Querying state before goXRPL launch...")
    for node in rippled_nodes:
        plan.request(
            service_name = node["name"],
            recipe = PostHttpRequestRecipe(
                port_id = "rpc",
                endpoint = "/",
                content_type = "application/json",
                body = '{"method": "server_info", "params": [{}]}',
            ),
        )

    # --- Step 2: Launch goXRPL ---
    plan.print("Launching goXRPL node...")
    goxrpl_nodes = goxrpl.launch(plan, 1, goxrpl_image, network_config)

    # --- Step 3: Wait for goXRPL to be ready, then observe ---
    plan.print("Waiting for goXRPL RPC to respond...")
    plan.wait(
        service_name = goxrpl_nodes[0]["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"seq": ".result.info.closed_ledger.seq"},
        ),
        field = "extract.seq",
        assertion = ">=",
        target_value = 2,
        timeout = "30s",
    )

    # --- Step 4: Wait 30s for sync attempt, then query all nodes ---
    plan.print("goXRPL is up. Waiting 30s for sync attempt...")

    # Use plan.wait with a higher seq target as a delay mechanism.
    # If goXRPL syncs, it will pass. If not, it times out and we still report.
    plan.print("Checking if goXRPL advances beyond seq 2 within 60s...")
    plan.wait(
        service_name = goxrpl_nodes[0]["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"seq": ".result.info.closed_ledger.seq"},
        ),
        field = "extract.seq",
        assertion = ">=",
        target_value = 3,
        timeout = "60s",
    )

    # --- Step 5: Final state report ---
    plan.print("=== Final state report ===")

    for node in rippled_nodes:
        plan.print("Querying " + node["name"] + "...")
        plan.request(
            service_name = node["name"],
            recipe = PostHttpRequestRecipe(
                port_id = "rpc",
                endpoint = "/",
                content_type = "application/json",
                body = '{"method": "server_info", "params": [{}]}',
            ),
        )

    for node in goxrpl_nodes:
        plan.print("Querying " + node["name"] + "...")
        plan.request(
            service_name = node["name"],
            recipe = PostHttpRequestRecipe(
                port_id = "rpc",
                endpoint = "/",
                content_type = "application/json",
                body = '{"method": "server_info", "params": [{}]}',
            ),
        )

    results["delayed_sync"] = "completed"
    return results
