"""Dashboard service definition."""

DASHBOARD_PORT = 8080

# rippled/goxrpl default ports. RPC serves HTTP JSON-RPC; WS serves the
# XRPL subscribe protocol, which the dashboard uses to get ledger-close
# events pushed in real time instead of polling.
RPC_PORT = 5005
WS_PORT = 6006

def launch(plan, rippled_nodes, goxrpl_nodes, dashboard_files):
    """Launch the monitoring dashboard.

    Args:
        plan: Kurtosis plan object.
        rippled_nodes: List of rippled node descriptors.
        goxrpl_nodes: List of goXRPL node descriptors.
        dashboard_files: Files artifact containing dashboard code.

    Returns:
        Dashboard service reference.
    """
    # Build config.json with node connection details.
    # Each entry carries both RPC (HTTP) and WS URLs — the dashboard uses
    # WS for push-driven ledger-close notifications and HTTP polling only
    # for static/slow-moving fields (peers, uptime, server_state).
    nodes_json = []
    for node in rippled_nodes:
        nodes_json.append(
            '{{"name":"{name}","type":"rippled","rpc":"http://{name}:{rpc}","ws":"ws://{name}:{ws}"}}'.format(
                name = node["name"], rpc = RPC_PORT, ws = WS_PORT,
            ),
        )
    for node in goxrpl_nodes:
        nodes_json.append(
            '{{"name":"{name}","type":"goxrpl","rpc":"http://{name}:{rpc}","ws":"ws://{name}:{ws}"}}'.format(
                name = node["name"], rpc = RPC_PORT, ws = WS_PORT,
            ),
        )

    # Poll cadence can now be slower — the latency-sensitive ledger-flip
    # field arrives over WS. Keep 5s for peers/state which are coarse.
    config_content = '{{"nodes":[{}],"poll_interval_ms":5000,"fuzz_metrics_url":"http://fuzz-soak:8081/metrics"}}'.format(",".join(nodes_json))

    config_artifact = plan.render_templates(
        name = "dashboard-config",
        config = {
            "config.json": struct(template = config_content, data = {}),
        },
    )

    service = plan.add_service(
        name = "dashboard",
        config = ServiceConfig(
            # node:22 exposes the WHATWG WebSocket API in the global
            # scope without experimental flags, which lets server.js
            # open XRPL subscribe streams without pulling npm deps.
            image = "node:22-alpine",
            ports = {
                "http": PortSpec(
                    number = DASHBOARD_PORT,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = {
                "/app": dashboard_files,
                "/app/config": config_artifact,
            },
            cmd = ["node", "/app/server.js"],
            env_vars = {
                "CONFIG_PATH": "/app/config/config.json",
                "PORT": str(DASHBOARD_PORT),
            },
        ),
    )

    plan.print("Dashboard available at http://{}:{}".format(service.ip_address, DASHBOARD_PORT))

    return service
