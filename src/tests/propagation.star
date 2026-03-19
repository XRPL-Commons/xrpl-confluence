"""Transaction propagation tests.

Verifies that transactions submitted to one node type are correctly
received and processed by nodes of the other type.
"""

helpers = import_module("../helpers/rpc.star")


def run(plan, nodes):
    """Run transaction propagation tests.

    Test scenarios:
    1. Submit tx to rippled -> verify goXRPL receives it
    2. Submit tx to goXRPL -> verify rippled receives it

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors.

    Returns:
        Test results dict.
    """
    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    goxrpl_nodes = [n for n in nodes if n["type"] == "goxrpl"]

    results = {}

    # Wait for the network to be live before submitting anything.
    plan.print("Waiting for network to be live (all nodes closed_seq >= 3)...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")
    plan.print("Network is live.")

    # Test 1: rippled -> goXRPL propagation
    plan.print("Test: tx submitted to rippled propagates to goXRPL")
    results["rippled_to_goxrpl"] = _test_rippled_to_goxrpl(
        plan, rippled_nodes, goxrpl_nodes,
    )

    # Test 2: goXRPL -> rippled propagation
    plan.print("Test: tx submitted to goXRPL propagates to rippled")
    results["goxrpl_to_rippled"] = _test_goxrpl_to_rippled(
        plan, rippled_nodes, goxrpl_nodes,
    )

    return results


def _test_rippled_to_goxrpl(plan, rippled_nodes, goxrpl_nodes):
    """Submit a Payment via rippled and verify goXRPL sees the result.

    Uses genesis account to send 100 XRP to TEST_DEST_1. After a few ledger
    closes, verifies the destination account exists on a goXRPL node.

    Args:
        plan: Kurtosis plan object.
        rippled_nodes: List of rippled node descriptors.
        goxrpl_nodes: List of goXRPL node descriptors.

    Returns:
        Test result string.
    """
    source = rippled_nodes[0]
    target = goxrpl_nodes[0]

    plan.print("  Submitting Payment from genesis via " + source["name"])
    plan.request(
        service_name = source["name"],
        recipe = helpers.submit_payment_recipe(
            secret = helpers.GENESIS_SECRET,
            account = helpers.GENESIS_ADDRESS,
            destination = helpers.TEST_DEST_1,
            amount = "100000000",
        ),
    )

    # Wait for a few ledger closes so the tx gets validated.
    plan.print("  Waiting for tx to be validated on " + target["name"] + "...")
    helpers.wait_for_ledger_seq(plan, target, 6, timeout = "60s")

    # Verify the destination account exists on the goXRPL node.
    plan.print("  Checking destination account on " + target["name"])
    plan.wait(
        service_name = target["name"],
        recipe = helpers.account_info_recipe(helpers.TEST_DEST_1),
        field = "extract.status",
        assertion = "==",
        target_value = "success",
        timeout = "60s",
    )

    plan.print("  PASS: Payment from rippled visible on goXRPL")
    return "passed"


def _test_goxrpl_to_rippled(plan, rippled_nodes, goxrpl_nodes):
    """Submit a Payment via goXRPL and verify rippled sees the result.

    Uses genesis account to send 100 XRP to TEST_DEST_2. After a few ledger
    closes, verifies the destination account exists on a rippled node.

    Args:
        plan: Kurtosis plan object.
        rippled_nodes: List of rippled node descriptors.
        goxrpl_nodes: List of goXRPL node descriptors.

    Returns:
        Test result string.
    """
    source = goxrpl_nodes[0]
    target = rippled_nodes[0]

    plan.print("  Submitting Payment from genesis via " + source["name"])
    plan.request(
        service_name = source["name"],
        recipe = helpers.submit_payment_recipe(
            secret = helpers.GENESIS_SECRET,
            account = helpers.GENESIS_ADDRESS,
            destination = helpers.TEST_DEST_2,
            amount = "100000000",
        ),
    )

    # Wait for a few ledger closes so the tx gets validated.
    plan.print("  Waiting for tx to be validated on " + target["name"] + "...")
    helpers.wait_for_ledger_seq(plan, target, 8, timeout = "60s")

    # Verify the destination account exists on the rippled node.
    plan.print("  Checking destination account on " + target["name"])
    plan.wait(
        service_name = target["name"],
        recipe = helpers.account_info_recipe(helpers.TEST_DEST_2),
        field = "extract.status",
        assertion = "==",
        target_value = "success",
        timeout = "60s",
    )

    plan.print("  PASS: Payment from goXRPL visible on rippled")
    return "passed"
