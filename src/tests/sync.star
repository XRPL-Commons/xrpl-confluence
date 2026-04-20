"""Ledger sync tests.

Verifies that nodes can sync ledger history from peers of a different
implementation type.
"""

goxrpl = import_module("../goxrpl/goxrpl.star")
helpers = import_module("../helpers/rpc.star")


def run(plan, nodes, goxrpl_image = None, network_config = None):
    """Run ledger sync tests.

    Test scenarios:
    1. Late-joining goXRPL node syncs from the existing mixed network.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.
        goxrpl_image: Docker image for goXRPL (needed to launch new node).
        network_config: Shared network configuration artifact.

    Returns:
        Test results dict.
    """
    results = {}

    if goxrpl_image == None or network_config == None:
        plan.print("SKIP: sync tests require goxrpl_image and network_config")
        results["late_join_sync"] = "skipped"
        return results

    plan.print("Test: late-joining goXRPL node syncs with mixed network")
    results["late_join_sync"] = _test_late_join_sync(
        plan, nodes, goxrpl_image, network_config,
    )

    return results


def _test_late_join_sync(plan, nodes, goxrpl_image, network_config):
    """Launch a fresh goXRPL node after the network has advanced and verify it syncs.

    1. Wait for the network to produce ledgers with some state (funded account).
    2. Launch a new goXRPL node.
    3. Verify it syncs and can serve the same ledger data.

    Args:
        plan: Kurtosis plan object.
        nodes: List of existing node descriptors.
        goxrpl_image: Docker image for goXRPL.
        network_config: Shared network configuration artifact.

    Returns:
        Test result string.
    """
    reference_node = nodes[0]

    # --- Phase 1: Let the network advance and create some state ---
    plan.print("  Waiting for network to reach closed_seq >= 5...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 5, timeout = "120s")

    # Submit a Payment to create ledger state beyond empty genesis.
    plan.print("  Submitting Payment to create state...")
    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    plan.request(
        service_name = submit_node["name"],
        recipe = helpers.submit_payment_recipe(
            secret = helpers.GENESIS_SECRET,
            account = helpers.GENESIS_ADDRESS,
            destination = helpers.TEST_DEST_1,
            amount = "200000000",
        ),
    )

    # Wait for the tx to be committed.
    plan.print("  Waiting for state to be committed (closed_seq >= 10)...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 10, timeout = "120s")

    # --- Phase 2: Launch a fresh goXRPL node ---
    plan.print("  Launching fresh goXRPL node...")
    new_nodes = goxrpl.launch(plan, 1, goxrpl_image, network_config, name_prefix = "goxrpl-late")
    new_node = new_nodes[0]

    # --- Phase 3: Wait for the new node to sync ---
    plan.print("  Waiting for " + new_node["name"] + " to sync (closed_seq >= 3)...")
    helpers.wait_for_ledger_seq(plan, new_node, 3, timeout = "60s")

    plan.print("  Waiting for " + new_node["name"] + " to reach closed_seq >= 15...")
    helpers.wait_for_ledger_seq(plan, new_node, 15, timeout = "120s")

    # --- Phase 4: Verify state consistency ---
    # Use ledger_index="validated" to dodge the race where a hardcoded
    # seq isn't closed/validated on one of the nodes yet. Both the
    # reference rippled node and the freshly-synced goXRPL must agree
    # on the same validated ledger hash — that's the real proof that
    # goXRPL processed the same history rippled did.
    plan.print("  Comparing validated ledger between reference and new node...")
    compare_nodes = [reference_node, new_node]
    helpers.assert_validated_ledgers_match(plan, compare_nodes)

    # Verify the new node can serve account data for the funded destination.
    plan.print("  Checking destination account on new node...")
    plan.wait(
        service_name = new_node["name"],
        recipe = helpers.account_info_recipe(helpers.TEST_DEST_1),
        field = "extract.status",
        assertion = "==",
        target_value = "success",
        timeout = "60s",
    )

    # Final state report
    plan.print("  Final state of all nodes:")
    helpers.query_all_server_info(plan, nodes + new_nodes)

    plan.print("  PASS: Late-joining goXRPL node synced and serves consistent state")
    return "passed"
