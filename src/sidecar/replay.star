"""Kurtosis service definition for the mainnet-replay sidecar."""


def launch(
    plan,
    all_nodes,
    submit_node,
    image = "xrpl-confluence-sidecar:latest",
    mainnet_url = "https://s1.ripple.com:51234",
    ledger_start = 80000000,
    ledger_end = 80000010,
    accounts = 10,
    batch_close = "5s",
    seed = None,
):
    """Launch the replay sidecar.

    Args:
        plan: Kurtosis plan object.
        all_nodes: List of all node descriptors.
        submit_node: Node descriptor to submit transactions to.
        image: Docker image containing /fuzz binary (default "xrpl-confluence-sidecar:latest").
        mainnet_url: Public rippled JSON-RPC endpoint to fetch ledgers from.
        ledger_start: First mainnet ledger index to replay (inclusive).
        ledger_end: Last mainnet ledger index to replay (inclusive).
        accounts: Number of test accounts to create.
        batch_close: Duration between layer-1 batch oracle checks.
        seed: uint64 fuzz seed. None → sidecar chooses crypto-random.

    Returns:
        The replay service reference.
    """
    node_urls = ",".join([
        "http://{}:5005".format(n["name"]) for n in all_nodes
    ])
    submit_url = "http://{}:5005".format(submit_node["name"])

    env = {
        "MODE":                "replay",
        "NODES":               node_urls,
        "SUBMIT_URL":          submit_url,
        "MAINNET_URL":         mainnet_url,
        "REPLAY_LEDGER_START": str(ledger_start),
        "REPLAY_LEDGER_END":   str(ledger_end),
        "ACCOUNTS":            str(accounts),
        "BATCH_CLOSE":         batch_close,
        "CORPUS_DIR":          "/output/corpus",
    }
    if seed != None:
        env["FUZZ_SEED"] = str(seed)

    return plan.add_service(
        name = "replay",
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
