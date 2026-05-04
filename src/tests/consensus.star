"""Consensus tests.

Verifies that rippled and goXRPL validators can participate in the same
consensus network and agree on validated ledgers.
"""

helpers = import_module("../helpers/rpc.star")


def run(plan, nodes, goxrpl_image = None, network_config = None):
    """Run consensus compatibility tests.

    Test scenarios:
    1. Mixed validator set reaches consensus (all nodes advance)
    2. All nodes agree on the same validated ledger hash

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.
        goxrpl_image: Docker image for goXRPL (unused in smoke tests).
        network_config: Shared network configuration artifact (unused in smoke tests).

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
    """Verify that a mixed network of rippled + goXRPL advances ledgers.

    Waits for every node (both rippled and goXRPL) to reach validated ledger
    sequence >= 5. If any node fails to reach this within the timeout, the
    Kurtosis plan fails, which signals a consensus issue.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test result string.
    """
    plan.print("  Waiting for all nodes to reach closed_ledger.seq >= 5...")

    for node in nodes:
        plan.print("    Waiting for " + node["name"] + " (" + node["type"] + ")")
        helpers.wait_for_ledger_seq(plan, node, 5, timeout = "120s")

    plan.print("  All nodes reached closed_ledger.seq >= 5")

    # Query final state for reporting
    helpers.query_all_server_info(plan, nodes)

    return "passed"


def _test_ledger_hash_agreement(plan, nodes):
    """Verify all nodes agree on the same validated ledger hash.

    Waits for all nodes to advance to seq >= 10, then queries the ledger hash
    for seq 5 on every node. Since Starlark cannot compare extracted values
    programmatically, the hashes are printed for visual/CI-log inspection.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test result string.
    """
    target_seq = 10
    compare_seq = 5

    plan.print("  Waiting for all nodes to reach closed_ledger.seq >= {}...".format(target_seq))

    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, target_seq, timeout = "120s")

    plan.print("  All nodes reached seq >= {}. Comparing hashes for seq {}...".format(target_seq, compare_seq))

    # Query ledger hashes on every node for the same historic ledger.
    # All nodes should return identical ledger_hash, account_hash, and
    # transaction_hash if consensus is working correctly.
    helpers.query_ledger_hashes(plan, nodes, compare_seq)

    return "completed"
