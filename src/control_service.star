"""Control service definition for confluence-control."""

CONTROL_PORT = 8090
RPC_PORT = 5005


def launch(plan, rippled_nodes, goxrpl_nodes, scenarios_artifact, image = "xrpl-confluence-sidecar:latest"):
    """Launch the confluence-control service.

    Args:
        plan: Kurtosis plan object.
        rippled_nodes: List of rippled node descriptors.
        goxrpl_nodes: List of go-xrpl node descriptors.
        scenarios_artifact: Files artifact containing scenario YAML files.
        image: Docker image containing /confluence-control binary.

    Returns:
        Control service reference.
    """
    nodes_json = []
    for node in rippled_nodes:
        nodes_json.append(
            '{{"name":"{name}","type":"rippled","rpc":"http://{name}:{rpc}"}}'.format(
                name = node["name"], rpc = RPC_PORT,
            ),
        )
    for node in goxrpl_nodes:
        nodes_json.append(
            '{{"name":"{name}","type":"goxrpl","rpc":"http://{name}:{rpc}"}}'.format(
                name = node["name"], rpc = RPC_PORT,
            ),
        )

    nodes_content = '{{"nodes":[{}]}}'.format(",".join(nodes_json))

    config_artifact = plan.render_templates(
        name = "control-config",
        config = {
            "nodes.json": struct(template = nodes_content, data = {}),
        },
    )

    service = plan.add_service(
        name = "confluence-control",
        config = ServiceConfig(
            image = image,
            # Override the image ENTRYPOINT (which is /fuzz). Kurtosis's `cmd`
            # is appended to ENTRYPOINT in Docker; `entrypoint` replaces it.
            entrypoint = ["/confluence-control"],
            cmd = [
                "--listen", ":{}".format(CONTROL_PORT),
                "--nodes-config", "/app/config/nodes.json",
                "--scenarios-dir", "/app/scenarios",
                "--poll-interval", "5s",
            ],
            ports = {
                "http": PortSpec(
                    number = CONTROL_PORT,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = {
                "/app/config":              config_artifact,
                "/app/scenarios":           scenarios_artifact,
                # Shared persistent volume with the fuzz-soak/fuzz-chaos
                # sidecar. The disk_watcher (--findings-dir default) tails
                # this dir for divergences mirrored from the fuzz corpus, so
                # `confluence findings` surfaces them via /v1/findings.
                "/var/confluence/findings": Directory(persistent_key = "confluence-findings"),
            },
        ),
    )

    plan.print("Control service available at http://{}:{}/v1/healthz".format(service.ip_address, CONTROL_PORT))

    return service
