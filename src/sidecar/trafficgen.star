"""Kurtosis service definition for the trafficgen sidecar.

The trafficgen sidecar generates diverse XRPL transactions against the mixed
network and compares ledger hashes across all nodes via a hash oracle.
"""


def launch(plan, all_nodes, submit_node, image = "trafficgen:latest", tx_count = 100, tx_mix = "payment:60,offer:20,trustset:10,accountset:10", accounts = 10, ledger_wait = 5):
    """Launch the traffic generator sidecar.

    Args:
        plan: Kurtosis plan object.
        all_nodes: List of all node descriptors.
        submit_node: Node descriptor to submit transactions to.
        image: Docker image for trafficgen (default "trafficgen:latest").
        tx_count: Total number of transactions to generate.
        tx_mix: Transaction type weights (e.g. "payment:60,offer:20").
        accounts: Number of test accounts to create.
        ledger_wait: Extra ledger closes to wait after last tx.

    Returns:
        The trafficgen service reference.
    """
    # Build comma-separated list of node addresses.
    node_addrs = ",".join([
        "{}:5005".format(n["name"]) for n in all_nodes
    ])

    submit_addr = "{}:5005".format(submit_node["name"])

    service = plan.add_service(
        name = "trafficgen",
        config = ServiceConfig(
            image = image,
            ports = {
                "results": PortSpec(
                    number = 8081,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            env_vars = {
                "NODES": node_addrs,
                "SUBMIT_NODE": submit_addr,
                "TX_COUNT": str(tx_count),
                "TX_MIX": tx_mix,
                "ACCOUNTS": str(accounts),
                "LEDGER_WAIT": str(ledger_wait),
            },
        ),
    )

    return service
