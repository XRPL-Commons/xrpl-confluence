"""rxrpl node service definition."""

PEER_PORT = 51235
RPC_PORT = 5005
WS_PORT = 6006

def launch(plan, count, image, network_config, name_prefix = "rxrpl"):
    """Launch rxrpl validator nodes.

    Args:
        plan: Kurtosis plan object.
        count: Number of rxrpl nodes to launch.
        image: Docker image for rxrpl.
        network_config: Shared network configuration artifact.
        name_prefix: Service name prefix (default: "rxrpl").

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
                "/etc/rxrpl": network_config,
            },
            cmd = ["server", "--config", "/etc/rxrpl/rxrpl-{}.toml".format(i)],
        )

    services = plan.add_services(configs)

    for name, service in services.items():
        nodes.append({
            "name": name,
            "type": "rxrpl",
            "service": service,
            "rpc_url": "http://{}:{}".format(service.ip_address, RPC_PORT),
            "ws_url": "ws://{}:{}".format(service.ip_address, WS_PORT),
            "peer_port": PEER_PORT,
        })

    return nodes
