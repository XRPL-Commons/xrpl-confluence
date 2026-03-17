"""
xrpl-confluence: XRPL multi-implementation interop testing harness.

Orchestrates mixed networks of rippled and goXRPL nodes to validate
p2p messaging, transaction propagation, ledger sync, and consensus compatibility.
"""

rippled = import_module("./src/rippled/rippled.star")
goxrpl = import_module("./src/goxrpl/goxrpl.star")
topology = import_module("./src/topology.star")
tests = import_module("./src/tests/tests.star")
dashboard = import_module("./src/dashboard/dashboard.star")

DEFAULT_RIPPLED_COUNT = 3
DEFAULT_GOXRPL_COUNT = 2

def run(plan, args = {}):
    """Spin up a mixed XRPL network and run interop tests.

    Args:
        plan: Kurtosis plan object.
        args: Configuration dictionary.
            - rippled_count: Number of rippled nodes (default: 3).
            - goxrpl_count: Number of goXRPL nodes (default: 2).
            - rippled_image: Docker image for rippled (default: "rippleci/rippled:latest").
            - goxrpl_image: Docker image for goXRPL (default: "goxrpl:latest").
            - test_suite: Which test suite to run: "all", "propagation", "sync", "consensus" (default: "all").
    """
    rippled_count = args.get("rippled_count", DEFAULT_RIPPLED_COUNT)
    goxrpl_count = args.get("goxrpl_count", DEFAULT_GOXRPL_COUNT)
    rippled_image = args.get("rippled_image", "rippleci/rippled:2.6.2")
    goxrpl_image = args.get("goxrpl_image", "goxrpl:latest")
    test_suite = args.get("test_suite", "all")

    plan.print("Starting xrpl-confluence with {} rippled + {} goXRPL nodes".format(rippled_count, goxrpl_count))

    # Generate shared network config (validator keys, peer list, genesis ledger)
    network_config = topology.generate_network_config(plan, rippled_count, goxrpl_count)

    # Launch rippled nodes
    rippled_nodes = rippled.launch(plan, rippled_count, rippled_image, network_config)

    # Launch goXRPL nodes
    goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config)

    # Launch monitoring dashboard
    dashboard_files = plan.upload_files(src = "./dashboard", name = "dashboard-files")
    dashboard.launch(plan, rippled_nodes, goxrpl_nodes, dashboard_files)

    # Run interop test suite
    all_nodes = rippled_nodes + goxrpl_nodes
    test_results = tests.run(plan, all_nodes, test_suite)

    return test_results
