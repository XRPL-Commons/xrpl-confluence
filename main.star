"""
xrpl-confluence: XRPL multi-implementation interop testing harness.

Orchestrates mixed networks of rippled and goXRPL nodes to validate
p2p messaging, transaction propagation, ledger sync, and consensus compatibility.
"""

rippled = import_module("./src/rippled/rippled.star")
goxrpl = import_module("./src/goxrpl/goxrpl.star")
topology = import_module("./src/topology.star")
tests = import_module("./src/tests/tests.star")
delayed_sync = import_module("./src/tests/delayed_sync.star")
dashboard = import_module("./src/dashboard/dashboard.star")

DEFAULT_RIPPLED_COUNT = 4
DEFAULT_GOXRPL_COUNT = 1

def run(plan, args = {}):
    """Spin up a mixed XRPL network and run interop tests.

    Args:
        plan: Kurtosis plan object.
        args: Configuration dictionary.
            - rippled_count: Number of rippled nodes (default: 4).
            - goxrpl_count: Number of goXRPL nodes (default: 1).
            - rippled_image: Docker image for rippled (default: "rippleci/rippled:2.6.2").
            - goxrpl_image: Docker image for goXRPL (default: "goxrpl:latest").
            - test_suite: Which test suite to run: "all", "propagation", "sync", "consensus", "soak", "delayed_sync", "fuzz", "replay", "shrink", "chaos" (default: "all").
            - shrink_args: For test_suite == "shrink": dict with shrink_artifact, shrink_max_step, optionally seed/accounts/validate_timeout.
            - soak_args: For test_suite == "soak": dict with tx_rate, rotate_every, mutation_rate, accounts, corpus_host_path.
            - chaos_args: For test_suite == "chaos": dict with schedule (JSON string, required), tx_rate, rotate_every, mutation_rate, accounts.
    """
    rippled_count = args.get("rippled_count", DEFAULT_RIPPLED_COUNT)
    goxrpl_count = args.get("goxrpl_count", DEFAULT_GOXRPL_COUNT)
    rippled_image = args.get("rippled_image", "rippleci/rippled:2.6.2")
    goxrpl_image = args.get("goxrpl_image", "goxrpl:latest")
    test_suite = args.get("test_suite", "all")
    shrink_args = args.get("shrink_args")

    plan.print("Starting xrpl-confluence with {} rippled + {} goXRPL nodes".format(rippled_count, goxrpl_count))

    # Generate shared network config (validator keys, peer list, genesis ledger)
    network_config = topology.generate_network_config(plan, rippled_count, goxrpl_count)

    # Upload dashboard files once (shared across all paths)
    dashboard_files = plan.upload_files(src = "./dashboard", name = "dashboard-files")

    # Launch rippled nodes
    rippled_nodes = rippled.launch(plan, rippled_count, rippled_image, network_config)

    # Delayed sync test: launch rippled first, start dashboard with rippled-only,
    # then run the test which launches goXRPL internally.
    if test_suite == "delayed_sync":
        dashboard.launch(plan, rippled_nodes, [], dashboard_files)
        plan.print("=== Running delayed sync test (goXRPL launches after rippled advances) ===")
        return delayed_sync.run(plan, rippled_nodes, goxrpl_image, network_config)

    # Launch goXRPL nodes. Chaos suite swaps in goxrpl-tools:latest so
    # iproute2/iptables are available for netem/partition events.
    enable_chaos_tools = (test_suite == "chaos")
    goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config, enable_chaos_tools = enable_chaos_tools)

    # Launch monitoring dashboard with all nodes
    dashboard.launch(plan, rippled_nodes, goxrpl_nodes, dashboard_files)

    # Run interop test suite
    all_nodes = rippled_nodes + goxrpl_nodes
    test_results = tests.run(plan, all_nodes, test_suite, goxrpl_image, network_config, shrink_args, args)

    return test_results
