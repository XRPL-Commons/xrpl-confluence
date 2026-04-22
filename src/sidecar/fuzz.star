"""Kurtosis service definition for the fuzz sidecar.

The fuzz sidecar generates transactions via xrpl-go, cross-checks per-tx
results (oracle layer 2) and ledger hashes (oracle layer 1), and records
divergences into a corpus directory.
"""


def launch(
    plan,
    all_nodes,
    submit_node,
    image = "trafficgen:latest",
    tx_count = 100,
    accounts = 10,
    batch_close = "5s",
    seed = None,
):
    """Launch the fuzz sidecar.

    Args:
        plan: Kurtosis plan object.
        all_nodes: List of all node descriptors.
        submit_node: Node descriptor to submit transactions to.
        image: Docker image containing /fuzz binary (default "trafficgen:latest").
        tx_count: Total number of transactions to submit.
        accounts: Number of test accounts to create.
        batch_close: Duration between layer-1 batch oracle checks.
        seed: uint64 fuzz seed. None → sidecar chooses crypto-random.

    Returns:
        The fuzz service reference.
    """
    node_urls = ",".join([
        "http://{}:5005".format(n["name"]) for n in all_nodes
    ])
    submit_url = "http://{}:5005".format(submit_node["name"])

    env = {
        "NODES":       node_urls,
        "SUBMIT_URL":  submit_url,
        "TX_COUNT":    str(tx_count),
        "ACCOUNTS":    str(accounts),
        "BATCH_CLOSE": batch_close,
        "CORPUS_DIR":  "/output/corpus",
    }
    if seed != None:
        env["FUZZ_SEED"] = str(seed)

    service = plan.add_service(
        name = "fuzz",
        config = ServiceConfig(
            image = image,
            entrypoint = ["/fuzz"],
            ports = {
                "results": PortSpec(
                    number = 8081,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            env_vars = env,
        ),
    )
    return service
