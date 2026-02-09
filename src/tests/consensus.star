"""Consensus tests.

Verifies that rippled and goXRPL validators can participate in the same
consensus network and agree on validated ledgers.
"""

def run(plan, nodes):
    """Run consensus compatibility tests.

    Test scenarios:
    1. Mixed validator set reaches consensus
    2. All nodes agree on the same validated ledger hash
    3. Network recovers after partition between impl types

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test results dict.
    """
    results = {}

    # Test 1: Mixed consensus
    plan.print("Test: mixed validator set reaches consensus")
    results["mixed_consensus"] = _test_mixed_consensus(plan, nodes)

    # Test 2: Ledger hash agreement
    plan.print("Test: all nodes agree on validated ledger hash")
    results["ledger_hash_agreement"] = _test_ledger_hash_agreement(plan, nodes)

    return results

def _test_mixed_consensus(plan, nodes):
    """Verify that a mixed network advances ledgers.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test result string.
    """
    # TODO: Implement
    # 1. Wait for N ledger closes
    # 2. Query server_info on all nodes
    # 3. Assert validated_ledger.seq is advancing on all nodes
    for node in nodes:
        plan.request(
            service_name = node["name"],
            recipe = PostHttpRequestRecipe(
                port_id = "rpc",
                endpoint = "/",
                content_type = "application/json",
                body = '{"method": "server_info", "params": [{}]}',
            ),
        )

    return "pending_implementation"

def _test_ledger_hash_agreement(plan, nodes):
    """Verify all nodes agree on the same validated ledger hash.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test result string.
    """
    # TODO: Implement
    # 1. Wait for ledger N to be validated on all nodes
    # 2. Query ledger hash for ledger N on all nodes
    # 3. Assert all hashes match

    return "pending_implementation"
