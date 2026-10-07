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
network_runtime = import_module("./src/network/network.star")

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
            - test_suite: Which test suite to run: "all", "propagation", "sync", "consensus", "soak", "delayed_sync", "fuzz", "replay", "shrink", "chaos", "none" (default: "all").
            - shrink_args: For test_suite == "shrink": dict with shrink_artifact, shrink_max_step, optionally seed/accounts/validate_timeout.
            - soak_args: For test_suite == "soak": dict with tx_rate, rotate_every, mutation_rate, accounts, corpus_host_path.
            - chaos_args: For test_suite == "chaos": dict with schedule (JSON string, required), tx_rate, rotate_every, mutation_rate, accounts.
    """
    test_suite = args.get("test_suite", "all")
    rippled_count = args.get("rippled_count", DEFAULT_RIPPLED_COUNT)
    goxrpl_count = args.get("goxrpl_count", 0 if test_suite == "none" else DEFAULT_GOXRPL_COUNT)
    rippled_image = args.get("rippled_image", "rippleci/rippled:2.6.2")
    goxrpl_image = args.get("goxrpl_image", "goxrpl:latest")
    shrink_args = args.get("shrink_args")
    network_settings = args.get("network_config")
    rippled_entrypoint = args.get("rippled_entrypoint")
    rippled_config = args.get("rippled_config")
    rippled_overrides = args.get("rippled_nodes")
    enable_dashboard = args.get("enable_dashboard", True)
    enable_control = args.get("enable_control", True)
    sidecar_image = args.get("sidecar_image", "xrpl-confluence-sidecar:latest")
    network_resume = args.get("network_resume", False)
    network_force_update = args.get("network_force_update", False)
    network_ready_timeout_seconds = args.get("network_ready_timeout_seconds", 180)

    # Pre-flight validation: catch impossible topologies before spending
    # minutes spinning up containers + waiting on consensus. Without this,
    # a bad arg combination (e.g. soak with goxrpl_count=0) gets caught
    # mid-run by a fail() inside a test suite — at which point Kurtosis
    # has already produced a partially-built enclave and the error reaches
    # the user *after* the network came up.
    _validate_topology(rippled_count, goxrpl_count, test_suite)
    _validate_image(rippled_image, "rippled_image")
    _validate_image(goxrpl_image, "goxrpl_image")
    _validate_runtime_args(
        test_suite,
        rippled_count,
        goxrpl_count,
        network_settings,
        rippled_config,
        rippled_overrides,
        network_resume,
        network_ready_timeout_seconds,
        enable_dashboard,
        enable_control,
        sidecar_image,
        network_force_update,
        rippled_entrypoint,
    )

    plan.print("Starting xrpl-confluence with {} rippled + {} go-xrpl nodes".format(rippled_count, goxrpl_count))

    # Generate shared network config (validator keys, peer list, genesis ledger).
    network_config = topology.generate_network_config(
        plan,
        rippled_count,
        goxrpl_count,
        network_config = network_settings,
        rippled_config = rippled_config,
        rippled_nodes = rippled_overrides,
    )

    dashboard_files = None
    scenarios_files = None
    if enable_dashboard:
        dashboard_files = plan.upload_files(src = "./dashboard", name = "dashboard-files")
    if enable_control:
        scenarios_files = plan.upload_files(src = "./scenarios", name = "control-scenarios")

    # Launch rippled nodes.  Keep the empty group out of add_services entirely
    # for all-go-xrpl legacy suites.
    rippled_nodes = []
    if rippled_count > 0:
        rippled_nodes = rippled.launch(
            plan,
            rippled_count,
            rippled_image,
            network_config,
            entrypoint = rippled_entrypoint,
            node_overrides = rippled_overrides,
            persistent = (test_suite == "none"),
            network_resume = network_resume,
            force_update = network_force_update,
        )

    dashboard_service = None
    control_service_ref = None

    # Delayed sync test: launch rippled first, start dashboard with rippled-only,
    # then run the test which launches go-xrpl internally.
    if test_suite == "delayed_sync":
        if enable_dashboard:
            dashboard_service = dashboard.launch(
                plan,
                rippled_nodes,
                [],
                dashboard_files,
                force_update = network_force_update,
            )
        # NOTE(M2.10): delayed_sync launches go-xrpl internally after rippled advances,
        # so goxrpl_nodes is [] here. The control service starts with rippled-only
        # and will not pick up go-xrpl nodes until live reconfig is added (M3+).
        if enable_control:
            control_service_ref = control_service.launch(
                plan,
                rippled_nodes,
                [],
                scenarios_files,
                image = sidecar_image,
                force_update = network_force_update,
            )
        plan.print("=== Running delayed sync test (go-xrpl launches after rippled advances) ===")
        return delayed_sync.run(plan, rippled_nodes, goxrpl_image, network_config)

    # Launch go-xrpl nodes. Chaos suite swaps in goxrpl-tools:latest so
    # iproute2/iptables are available for netem/partition events.
    enable_chaos_tools = (test_suite == "chaos")
    goxrpl_nodes = []
    if goxrpl_count > 0:
        goxrpl_nodes = goxrpl.launch(
            plan,
            goxrpl_count,
            goxrpl_image,
            network_config,
            enable_chaos_tools = enable_chaos_tools,
            force_update = network_force_update,
        )

    # Launch monitoring dashboard with all nodes
    if enable_dashboard:
        dashboard_service = dashboard.launch(
            plan,
            rippled_nodes,
            goxrpl_nodes,
            dashboard_files,
            force_update = network_force_update,
        )
    if enable_control:
        control_service_ref = control_service.launch(
            plan,
            rippled_nodes,
            goxrpl_nodes,
            scenarios_files,
            image = sidecar_image,
            force_update = network_force_update,
        )

    all_nodes = rippled_nodes + goxrpl_nodes
    if test_suite == "none":
        readiness = network_runtime.wait_for_readiness(
            plan,
            all_nodes,
            timeout_seconds = network_ready_timeout_seconds,
        )
        return _network_only_result(all_nodes, dashboard_service, control_service_ref, readiness)

    # Run interop test suite
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
    "delayed_sync": (1, 1),
}


def _validate_topology(rippled_count, goxrpl_count, test_suite):
    supported_suites = [
        "all", "propagation", "sync", "consensus", "soak", "delayed_sync",
        "fuzz", "replay", "shrink", "chaos", "none",
    ]
    if test_suite not in supported_suites:
        fail("unknown test_suite {!r}; expected one of {}".format(test_suite, ", ".join(supported_suites)))
    if type(rippled_count) != "int" or type(goxrpl_count) != "int":
        fail("rippled_count and goxrpl_count must be integers")
    if rippled_count < 0 or goxrpl_count < 0:
        fail("rippled_count and goxrpl_count must be >= 0 (got {} / {})".format(
            rippled_count, goxrpl_count,
        ))
    total = rippled_count + goxrpl_count
    if total < 2:
        fail("xrpl-confluence needs >= 2 total nodes (got {} rippled + {} goxrpl)".format(
            rippled_count, goxrpl_count,
        ))
    if total > 10:
        fail("xrpl-confluence supports at most 10 total nodes (got {})".format(total))
    if test_suite == "none":
        return
    mins = _SUITE_MIN_COUNTS.get(test_suite)
    if mins == None:
        return
    min_rippled, min_goxrpl = mins
    if rippled_count < min_rippled or goxrpl_count < min_goxrpl:
        fail("test_suite=\"{}\" requires >= {} rippled and >= {} goxrpl (got {} rippled, {} goxrpl)".format(
            test_suite, min_rippled, min_goxrpl, rippled_count, goxrpl_count,
        ))


def _validate_runtime_args(
    test_suite,
    rippled_count,
    goxrpl_count,
    network_settings,
    rippled_config,
    rippled_overrides,
    network_resume,
    network_ready_timeout_seconds,
    enable_dashboard,
    enable_control,
    sidecar_image,
    network_force_update,
    rippled_entrypoint,
):
    if type(enable_dashboard) != "bool" or type(enable_control) != "bool":
        fail("enable_dashboard and enable_control must be booleans")
    if type(sidecar_image) != "string" or sidecar_image.strip() == "" or sidecar_image.strip() != sidecar_image:
        fail("sidecar_image must be a non-empty image reference")
    if type(network_resume) != "bool" or type(network_force_update) != "bool":
        fail("network_resume and network_force_update must be booleans")
    if test_suite != "none" and not enable_control:
        fail("enable_control=false is supported only for test_suite=\"none\"")
    if type(network_settings) not in ["NoneType", "dict"]:
        fail("network_config must be an object")
    if type(rippled_config) not in ["NoneType", "dict"]:
        fail("rippled_config must be an object")
    if type(rippled_overrides) not in ["NoneType", "list"]:
        fail("rippled_nodes must be a list")
    _validate_entrypoint(rippled_entrypoint, "rippled_entrypoint")
    if type(network_ready_timeout_seconds) not in ["int", "float"]:
        fail("network_ready_timeout_seconds must be numeric")
    if network_ready_timeout_seconds <= 0:
        fail("network_ready_timeout_seconds must be positive (got {})".format(network_ready_timeout_seconds))
    if network_resume and (test_suite != "none" or goxrpl_count != 0):
        fail("network_resume requires test_suite=\"none\" with goxrpl_count=0")
    custom_network = network_settings != None and len(network_settings) > 0
    custom_rippled_config = rippled_config != None and len(rippled_config) > 0
    if rippled_overrides != None:
        if len(rippled_overrides) != rippled_count:
            fail("rippled_nodes must contain exactly {} entries (got {})".format(rippled_count, len(rippled_overrides)))
        for index, override in enumerate(rippled_overrides):
            if type(override) != "dict":
                fail("rippled_nodes[{}] must be an object".format(index))
            if "name" in override and override["name"] != "rippled-{}".format(index):
                fail("rippled_nodes[{}].name must be rippled-{}".format(index, index))
            if "image" in override:
                _validate_image(override["image"], "rippled_nodes[{}].image".format(index))
            if "config" in override and override["config"] == None:
                fail("rippled_nodes[{}].config cannot be null".format(index))
            if "config" in override and type(override["config"]) != "dict":
                fail("rippled_nodes[{}].config must be an object".format(index))
            if "config" in override and len(override["config"]) > 0:
                custom_rippled_config = True
            if "entrypoint" in override and override["entrypoint"] == None:
                fail("rippled_nodes[{}].entrypoint cannot be null".format(index))
            if "entrypoint" in override:
                _validate_entrypoint(override["entrypoint"], "rippled_nodes[{}].entrypoint".format(index))
    if (custom_network or custom_rippled_config) and (test_suite != "none" or goxrpl_count != 0):
        fail("custom network/config is currently supported only for test_suite=\"none\" with goxrpl_count=0")


def _validate_entrypoint(entrypoint, field_name):
    if entrypoint == None:
        return
    if type(entrypoint) != "list":
        fail("{} must be a list of command arguments".format(field_name))
    if len(entrypoint) == 0:
        fail("{} must contain at least one command argument".format(field_name))
    for index, arg in enumerate(entrypoint):
        if type(arg) != "string":
            fail("{}[{}] must be a string".format(field_name, index))
        if arg.strip() == "":
            fail("{}[{}] must be non-empty".format(field_name, index))


def _validate_image(image, field_name):
    if type(image) != "string" or image.strip() == "" or image.strip() != image:
        fail("{} must be non-empty and must not contain surrounding whitespace".format(field_name))


def _network_only_result(nodes, dashboard_service, control_service_ref, readiness):
    endpoints = {"nodes": []}
    for node in nodes:
        endpoints["nodes"].append({
            "name": node["name"],
            "type": node["type"],
            "rpc_url": node["rpc_url"],
            "ws_url": node["ws_url"],
            "peer_url": node.get("peer_url"),
        })
    if dashboard_service != None:
        endpoints["dashboard_url"] = "http://{}:{}".format(dashboard_service.ip_address, 8080)
    if control_service_ref != None:
        endpoints["control_url"] = "http://{}:{}/v1/healthz".format(control_service_ref.ip_address, 8090)
    return {
        "mode": "none",
        "nodes": nodes,
        "endpoints": endpoints,
        "readiness": readiness,
    }
