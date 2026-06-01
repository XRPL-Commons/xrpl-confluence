"""
xrpl-confluence: XRPL multi-implementation interop testing harness.

Orchestrates mixed networks of rippled and go-xrpl nodes to validate
p2p messaging, transaction propagation, ledger sync, and consensus compatibility.
"""

rippled = import_module("./src/rippled/rippled.star")
goxrpl = import_module("./src/goxrpl/goxrpl.star")
topology = import_module("./src/topology.star")
tests = import_module("./src/tests/tests.star")
delayed_sync = import_module("./src/tests/delayed_sync.star")
dashboard = import_module("./src/dashboard/dashboard.star")
control_service = import_module("./src/control_service.star")

DEFAULT_RIPPLED_COUNT = 4
DEFAULT_GOXRPL_COUNT = 1

def run(plan, args = {}):
    """Spin up a mixed XRPL network and run interop tests.

    Args:
        plan: Kurtosis plan object.
        args: Configuration dictionary.
            - rippled_count: Number of rippled nodes (default: 4).
            - goxrpl_count: Number of go-xrpl nodes (default: 1).
            - rippled_image: Docker image for rippled (default: "rippleci/rippled:2.6.2").
            - goxrpl_image: Docker image for go-xrpl (default: "goxrpl:latest").
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

    # Pre-flight validation: catch impossible topologies before spending
    # minutes spinning up containers + waiting on consensus. Without this,
    # a bad arg combination (e.g. soak with goxrpl_count=0) gets caught
    # mid-run by a fail() inside a test suite — at which point Kurtosis
    # has already produced a partially-built enclave and the error reaches
    # the user *after* the network came up.
    _validate_topology(rippled_count, goxrpl_count, test_suite)

    plan.print("Starting xrpl-confluence with {} rippled + {} go-xrpl nodes".format(rippled_count, goxrpl_count))

    # Generate shared network config (validator keys, peer list, genesis ledger)
    network_config = topology.generate_network_config(plan, rippled_count, goxrpl_count)

    # Upload dashboard files once (shared across all paths)
    dashboard_files = plan.upload_files(src = "./dashboard", name = "dashboard-files")
    scenarios_files = plan.upload_files(src = "./scenarios", name = "control-scenarios")

    # Launch rippled nodes
    rippled_nodes = rippled.launch(plan, rippled_count, rippled_image, network_config)

    # Delayed sync test: launch rippled first, start dashboard with rippled-only,
    # then run the test which launches go-xrpl internally.
    if test_suite == "delayed_sync":
        dashboard.launch(plan, rippled_nodes, [], dashboard_files)
        # NOTE(M2.10): delayed_sync launches go-xrpl internally after rippled advances,
        # so goxrpl_nodes is [] here. The control service starts with rippled-only
        # and will not pick up go-xrpl nodes until live reconfig is added (M3+).
        control_service.launch(plan, rippled_nodes, [], scenarios_files)
        plan.print("=== Running delayed sync test (go-xrpl launches after rippled advances) ===")
        return delayed_sync.run(plan, rippled_nodes, goxrpl_image, network_config)

    # Launch go-xrpl nodes. Chaos suite swaps in goxrpl-tools:latest so
    # iproute2/iptables are available for netem/partition events.
    enable_chaos_tools = (test_suite == "chaos")
    goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config, enable_chaos_tools = enable_chaos_tools)

    # Launch monitoring dashboard with all nodes
    dashboard.launch(plan, rippled_nodes, goxrpl_nodes, dashboard_files)
    control_service.launch(plan, rippled_nodes, goxrpl_nodes, scenarios_files)

    # Run interop test suite
    all_nodes = rippled_nodes + goxrpl_nodes
    test_results = tests.run(plan, all_nodes, test_suite, goxrpl_image, network_config, shrink_args, args)

    return test_results


# Per-suite minimum-counts table. Each entry maps a suite name to
# (min_rippled, min_goxrpl). Anything not listed has no minimum.
#
# Soak / chaos drive go-xrpl hard against multiple rippled validators, so
# both require >= 2 rippled and >= 1 goxrpl. Shrink replays a saved run
# log against the network and has the same minimums for the same reason.
# fuzz / replay are bounded but still need a peer for the oracle to compare
# against, hence >= 2 rippled.
_SUITE_MIN_COUNTS = {
    "soak":   (2, 1),
    "chaos":  (2, 1),
    "shrink": (2, 1),
    "fuzz":   (2, 0),
    "replay": (2, 0),
}


def _validate_topology(rippled_count, goxrpl_count, test_suite):
    if rippled_count < 0 or goxrpl_count < 0:
        fail("rippled_count and goxrpl_count must be >= 0 (got {} / {})".format(
            rippled_count, goxrpl_count,
        ))
    if rippled_count + goxrpl_count < 2:
        fail("xrpl-confluence needs >= 2 total nodes for oracle comparison (got {} rippled + {} goxrpl)".format(
            rippled_count, goxrpl_count,
        ))
    mins = _SUITE_MIN_COUNTS.get(test_suite)
    if mins == None:
        return
    min_rippled, min_goxrpl = mins
    if rippled_count < min_rippled or goxrpl_count < min_goxrpl:
        fail("test_suite=\"{}\" requires >= {} rippled and >= {} goxrpl (got {} rippled, {} goxrpl)".format(
            test_suite, min_rippled, min_goxrpl, rippled_count, goxrpl_count,
        ))
