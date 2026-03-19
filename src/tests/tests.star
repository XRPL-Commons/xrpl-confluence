"""Test suite orchestrator for xrpl-confluence."""

propagation = import_module("./propagation.star")
sync = import_module("./sync.star")
consensus = import_module("./consensus.star")

def run(plan, nodes, suite = "all", goxrpl_image = None, network_config = None):
    """Run the specified interop test suite.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors (rippled + goXRPL).
        suite: Which suite to run - "all", "propagation", "sync", "consensus", "soak".
        goxrpl_image: Docker image for goXRPL (needed by sync tests to launch new nodes).
        network_config: Shared network configuration artifact.

    Returns:
        Dict of test results.
    """
    results = {}

    if suite == "all" or suite == "propagation":
        plan.print("=== Running transaction propagation tests ===")
        results["propagation"] = propagation.run(plan, nodes)

    if suite == "all" or suite == "sync":
        plan.print("=== Running ledger sync tests ===")
        results["sync"] = sync.run(plan, nodes, goxrpl_image, network_config)

    if suite == "all" or suite == "consensus":
        plan.print("=== Running consensus tests ===")
        results["consensus"] = consensus.run(plan, nodes, goxrpl_image, network_config)

    if suite == "soak":
        plan.print("=== Running consensus soak test ===")
        results["soak"] = consensus.run_soak(plan, nodes)

    return results
