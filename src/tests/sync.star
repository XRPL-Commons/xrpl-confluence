"""Ledger sync tests.

Verifies that nodes can sync ledger history from peers of a different
implementation type.
"""

def run(plan, nodes):
    """Run ledger sync tests.

    Test scenarios:
    1. goXRPL syncs from rippled peers
    2. rippled syncs from goXRPL peers
    3. Late-joining node catches up with mixed network

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test results dict.
    """
    results = {}

    # TODO: Implement sync tests
    # 1. Let the network produce N ledgers
    # 2. Launch a fresh node of each type
    # 3. Verify it syncs to the same ledger hash as the rest of the network

    plan.print("Test: goXRPL syncs ledger history from rippled")
    results["goxrpl_syncs_from_rippled"] = "pending_implementation"

    plan.print("Test: rippled syncs ledger history from goXRPL")
    results["rippled_syncs_from_goxrpl"] = "pending_implementation"

    plan.print("Test: late-joining node catches up with mixed network")
    results["late_join_sync"] = "pending_implementation"

    return results
