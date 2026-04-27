"""Test suite orchestrator for xrpl-confluence."""

propagation = import_module("./propagation.star")
sync = import_module("./sync.star")
consensus = import_module("./consensus.star")
fuzz = import_module("./fuzz.star")
replay = import_module("./replay.star")
shrink = import_module("./shrink.star")

def run(plan, nodes, suite = "all", goxrpl_image = None, network_config = None, shrink_args = None):
    """Run the specified interop test suite.

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors (rippled + goXRPL).
        suite: Which suite to run - "all", "propagation", "sync", "consensus", "soak", "fuzz", "replay", "shrink".
        goxrpl_image: Docker image for goXRPL (needed by sync tests to launch new nodes).
        network_config: Shared network configuration artifact.
        shrink_args: Dict with shrink-suite inputs: shrink_artifact, shrink_max_step, optionally seed/accounts/validate_timeout.

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

    if suite == "fuzz":
        plan.print("=== Running fuzz suite ===")
        results["fuzz"] = fuzz.run(plan, nodes)

    if suite == "replay":
        plan.print("=== Running replay suite ===")
        results["replay"] = replay.run(plan, nodes)

    if suite == "shrink":
        if shrink_args == None:
            fail("shrink suite requires shrink_args (shrink_artifact, shrink_max_step)")
        plan.print("=== Running shrink suite ===")
        results["shrink"] = shrink.run(
            plan,
            nodes,
            shrink_artifact = shrink_args["shrink_artifact"],
            shrink_max_step = shrink_args["shrink_max_step"],
            accounts = shrink_args.get("accounts", 10),
            seed = shrink_args.get("seed"),
            validate_timeout = shrink_args.get("validate_timeout", "60s"),
        )

    return results
