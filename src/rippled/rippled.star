"""Rippled node service definition."""

PEER_PORT = 51235
RPC_PORT = 5005
WS_PORT = 6006

def launch(
    plan,
    count,
    image,
    network_config,
    entrypoint = None,
    node_overrides = None,
    persistent = False,
    network_resume = False,
    force_update = False,
):
    """Launch rippled validator nodes.

    Args:
        plan: Kurtosis plan object.
        count: Number of rippled nodes to launch.
        image: Docker image for rippled.
        network_config: Shared network configuration artifact.

    Returns:
        List of node descriptors with service references.
    """
    if type(count) != "int":
        fail("rippled count must be an integer")
    if count <= 0:
        return []
    if type(image) != "string" or image.strip() == "" or image.strip() != image:
        fail("rippled image must be non-empty")
    if node_overrides != None:
        if type(node_overrides) != "list":
            fail("rippled node overrides must be a list")
        if len(node_overrides) != count:
            fail("rippled node overrides must contain exactly {} entries (got {})".format(count, len(node_overrides)))

    nodes = []
    configs = {}

    for i in range(count):
        name = "rippled-{}".format(i)
        override = {} if node_overrides == None else node_overrides[i]
        if type(override) != "dict":
            fail("rippled node override {} must be an object".format(i))
        if "name" in override and override["name"] != name:
            fail("rippled_nodes[{}].name must be {}".format(i, name))
        node_image = override.get("image", image)
        if type(node_image) != "string" or node_image.strip() == "" or node_image.strip() != node_image:
            fail("rippled-{} image must be non-empty when supplied".format(i))
        node_entrypoint = entrypoint
        if "entrypoint" in override:
            node_entrypoint = override["entrypoint"]
        if node_entrypoint != None:
            if type(node_entrypoint) != "list" or len(node_entrypoint) == 0:
                fail("rippled-{} entrypoint must contain at least one command argument".format(i))
            for arg in node_entrypoint:
                if type(arg) != "string" or arg.strip() == "":
                    fail("rippled-{} entrypoint arguments must be non-empty strings".format(i))
        files = {
            "/etc/rippled": network_config,
        }
        if persistent:
            files["/var/lib/rippled/db"] = Directory(
                persistent_key = "rippled-{}-data".format(i),
            )
        start_flag = "--load" if network_resume else "--start"
        if node_entrypoint == None:
            configs[name] = ServiceConfig(
                image = node_image,
                ports = {
                    "peer": PortSpec(number = PEER_PORT, transport_protocol = "TCP"),
                    "rpc": PortSpec(number = RPC_PORT, transport_protocol = "TCP", application_protocol = "http"),
                    "ws": PortSpec(number = WS_PORT, transport_protocol = "TCP"),
                },
                files = files,
                cmd = ["--conf", "/etc/rippled/rippled-{}.cfg".format(i), start_flag],
                labels = {"fuzzer.role": "node"},
            )
        else:
            configs[name] = ServiceConfig(
                image = node_image,
                entrypoint = node_entrypoint,
                ports = {
                    "peer": PortSpec(number = PEER_PORT, transport_protocol = "TCP"),
                    "rpc": PortSpec(number = RPC_PORT, transport_protocol = "TCP", application_protocol = "http"),
                    "ws": PortSpec(number = WS_PORT, transport_protocol = "TCP"),
                },
                files = files,
                cmd = ["--conf", "/etc/rippled/rippled-{}.cfg".format(i), start_flag],
                labels = {"fuzzer.role": "node"},
            )

    services = plan.add_services(configs, force_update = force_update)

    for name, service in services.items():
        nodes.append({
            "name": name,
            "type": "rippled",
            "service": service,
            "rpc_url": "http://{}:{}".format(service.ip_address, RPC_PORT),
            "ws_url": "ws://{}:{}".format(service.ip_address, WS_PORT),
            "peer_url": "{}:{}".format(service.ip_address, PEER_PORT),
            "peer_port": PEER_PORT,
        })

    return nodes
