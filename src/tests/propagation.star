"""Transaction propagation tests.

Verifies that transactions submitted to one node type are correctly
received and processed by nodes of the other type.
"""

def run(plan, nodes):
    """Run transaction propagation tests.

    Test scenarios:
    1. Submit tx to rippled -> verify goXRPL receives it
    2. Submit tx to goXRPL -> verify rippled receives it
    3. Submit tx to rippled -> verify all nodes see it in validated ledger

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test results dict.
    """
    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    goxrpl_nodes = [n for n in nodes if n["type"] == "goxrpl"]

    results = {}

    # Test 1: rippled -> goXRPL propagation
    plan.print("Test: tx submitted to rippled propagates to goXRPL")
    results["rippled_to_goxrpl"] = _test_cross_propagation(
        plan, rippled_nodes[0], goxrpl_nodes,
    )

    # Test 2: goXRPL -> rippled propagation
    plan.print("Test: tx submitted to goXRPL propagates to rippled")
    results["goxrpl_to_rippled"] = _test_cross_propagation(
        plan, goxrpl_nodes[0], rippled_nodes,
    )

    return results

def _test_cross_propagation(plan, source_node, target_nodes):
    """Submit a transaction to source and verify targets see it.

    Args:
        plan: Kurtosis plan object.
        source_node: Node to submit the transaction to.
        target_nodes: Nodes that should receive the transaction.

    Returns:
        Test result string.
    """
    # Submit a Payment transaction via RPC
    submit_response = plan.request(
        service_name = source_node["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "submit", "params": [{"tx_blob": "PLACEHOLDER"}]}',
        ),
    )

    # Verify each target node sees the transaction
    # TODO: wait for validated ledger, then check tx via `tx` RPC method
    for target in target_nodes:
        plan.request(
            service_name = target["name"],
            recipe = PostHttpRequestRecipe(
                port_id = "rpc",
                endpoint = "/",
                content_type = "application/json",
                body = '{"method": "server_info", "params": [{}]}',
            ),
        )

    return "pending_implementation"
