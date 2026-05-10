"""
xrpl-confluence: XRPL multi-implementation interop testing harness.

Orchestrates mixed networks of rippled, goXRPL and rxrpl nodes to validate
p2p messaging, transaction propagation, ledger sync, and consensus compatibility.
"""

rippled = import_module("./src/rippled/rippled.star")
goxrpl = import_module("./src/goxrpl/goxrpl.star")
rxrpl = import_module("./src/rxrpl/rxrpl.star")
topology = import_module("./src/topology.star")
tests = import_module("./src/tests/tests.star")
delayed_sync = import_module("./src/tests/delayed_sync.star")
dashboard = import_module("./src/dashboard/dashboard.star")

DEFAULT_RIPPLED_COUNT = 4
DEFAULT_GOXRPL_COUNT = 1
DEFAULT_RXRPL_COUNT = 0

def run(plan, args = {}):
    """Spin up a mixed XRPL network and run interop tests.

    Args:
        plan: Kurtosis plan object.
        args: Configuration dictionary.
            - rippled_count: Number of rippled nodes (default: 4).
            - goxrpl_count: Number of goXRPL nodes (default: 1).
            - rxrpl_count: Number of rxrpl nodes (default: 0).
            - rippled_image: Docker image for rippled (default: "rippleci/rippled:2.6.2").
            - goxrpl_image: Docker image for goXRPL (default: "goxrpl:latest").
            - rxrpl_image: Docker image for rxrpl (default: "rxrpl:latest").
            - test_suite: Which test suite to run: "all", "propagation", "sync", "consensus", "soak", "delayed_sync", "fuzz", "replay" (default: "all").
    """
    rippled_count = args.get("rippled_count", DEFAULT_RIPPLED_COUNT)
    goxrpl_count = args.get("goxrpl_count", DEFAULT_GOXRPL_COUNT)
    rxrpl_count = args.get("rxrpl_count", DEFAULT_RXRPL_COUNT)
    rippled_image = args.get("rippled_image", "rippleci/rippled:2.6.2")
    goxrpl_image = args.get("goxrpl_image", "goxrpl:latest")
    rxrpl_image = args.get("rxrpl_image", "rxrpl:latest")
    test_suite = args.get("test_suite", "all")

    plan.print("Starting xrpl-confluence with {} rippled + {} goXRPL + {} rxrpl nodes".format(
        rippled_count, goxrpl_count, rxrpl_count,
    ))

    # Generate shared network config (validator keys, peer list, genesis ledger)
    network_config = topology.generate_network_config(plan, rippled_count, goxrpl_count, rxrpl_count)

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

    # Launch goXRPL nodes
    goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config)

    # Launch rxrpl nodes
    rxrpl_nodes = rxrpl.launch(plan, rxrpl_count, rxrpl_image, network_config)

    # Launch monitoring dashboard with all nodes
    dashboard.launch(plan, rippled_nodes, goxrpl_nodes + rxrpl_nodes, dashboard_files)

    # Run interop test suite
    all_nodes = rippled_nodes + goxrpl_nodes + rxrpl_nodes
    test_results = tests.run(plan, all_nodes, test_suite, goxrpl_image, network_config)

    return test_results
