"""goXRPL node service definition."""

PEER_PORT = 51235
RPC_PORT = 5005
WS_PORT = 6006

def launch(plan, count, image, network_config, name_prefix = "goxrpl"):
    """Launch goXRPL validator nodes.

    Args:
        plan: Kurtosis plan object.
        count: Number of goXRPL nodes to launch.
        image: Docker image for goXRPL.
        network_config: Shared network configuration artifact.
        name_prefix: Service name prefix (default: "goxrpl").

    Returns:
        List of node descriptors with service references.
    """
    nodes = []
    configs = {}

    for i in range(count):
        name = "{}-{}".format(name_prefix, i)
        configs[name] = ServiceConfig(
            image = image,
            ports = {
                "peer": PortSpec(number = PEER_PORT, transport_protocol = "TCP", wait = None),
                "rpc": PortSpec(number = RPC_PORT, transport_protocol = "TCP", application_protocol = "http"),
                "ws": PortSpec(number = WS_PORT, transport_protocol = "TCP"),
            },
            files = {
                "/etc/goxrpl": network_config,
            },
            cmd = ["server", "--conf", "/etc/goxrpl/goxrpl-{}.toml".format(i)],
        )

    services = plan.add_services(configs)

    for name, service in services.items():
        nodes.append({
            "name": name,
            "type": "goxrpl",
            "service": service,
            "rpc_url": "http://{}:{}".format(service.ip_address, RPC_PORT),
            "ws_url": "ws://{}:{}".format(service.ip_address, WS_PORT),
            "peer_port": PEER_PORT,
        })

    return nodes
