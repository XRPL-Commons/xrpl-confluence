"""Dashboard service definition."""

DASHBOARD_PORT = 8080

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
    # Build config.json with node connection details
    nodes_json = []
    for node in rippled_nodes:
        nodes_json.append('{{"name":"{}","type":"rippled","rpc":"http://{}:5005"}}'.format(
            node["name"], node["name"],
        ))
    for node in goxrpl_nodes:
        nodes_json.append('{{"name":"{}","type":"goxrpl","rpc":"http://{}:5005"}}'.format(
            node["name"], node["name"],
        ))

    config_content = '{{"nodes":[{}],"poll_interval_ms":2000}}'.format(",".join(nodes_json))

    config_artifact = plan.render_templates(
        name = "dashboard-config",
        config = {
            "config.json": struct(template = config_content, data = {}),
        },
    )

    service = plan.add_service(
        name = "dashboard",
        config = ServiceConfig(
            image = "node:20-alpine",
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
