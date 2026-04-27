"""Kurtosis service definition for the fuzz sidecar.

The fuzz sidecar generates transactions via xrpl-go, cross-checks per-tx
results (oracle layer 2) and ledger hashes (oracle layer 1), and records
divergences into a corpus directory.

For shrink mode, the caller pre-uploads the run log + divergence JSON as a
single Kurtosis files artifact named via `shrink_artifact`; this launcher
mounts that artifact at `/shrink-input/` and points SHRINK_LOG and
SHRINK_DIVERGENCE at fixed paths inside it.
"""


def launch(
    plan,
    all_nodes,
    submit_node,
    image = "trafficgen:latest",
    mode = "fuzz",
    tx_count = 100,
    accounts = 10,
    batch_close = "5s",
    seed = None,
    shrink_artifact = None,
    shrink_max_step = None,
    shrink_validate_timeout = "60s"):
    """Launch the fuzz sidecar.

    Args:
        plan: Kurtosis plan object.
        all_nodes: List of all node descriptors.
        submit_node: Node descriptor to submit transactions to.
        image: Docker image containing /fuzz binary (default "trafficgen:latest").
        mode: One of "fuzz" (default), "shrink". Other modes (replay/reproduce)
            currently use their own launcher; shrink is wired here because it
            shares everything with fuzz mode except the input files.
        tx_count: fuzz mode: total number of transactions to submit.
        accounts: Number of test accounts to create.
        batch_close: Duration between layer-1 batch oracle checks.
        seed: uint64 fuzz seed. None -> sidecar chooses crypto-random.
        shrink_artifact: shrink mode: name of a Kurtosis files artifact
            containing exactly `run.ndjson` and `div.json` at its root.
            Mounted at /shrink-input/.
        shrink_max_step: shrink mode: inclusive prefix cap on RunLogEntry.Step.
        shrink_validate_timeout: shrink mode: per-tx wait for validated:true.

    Returns:
        The fuzz service reference.
    """
    node_urls = ",".join([
        "http://{}:5005".format(n["name"]) for n in all_nodes
    ])
    submit_url = "http://{}:5005".format(submit_node["name"])

    env = {
        "MODE":        mode,
        "NODES":       node_urls,
        "SUBMIT_URL":  submit_url,
        "ACCOUNTS":    str(accounts),
        "BATCH_CLOSE": batch_close,
        "CORPUS_DIR":  "/output/corpus",
    }
    files = {}

    if mode == "fuzz":
        env["TX_COUNT"] = str(tx_count)
    elif mode == "shrink":
        if shrink_artifact == None or shrink_max_step == None:
            fail("shrink mode requires shrink_artifact and shrink_max_step")
        env["SHRINK_LOG"] = "/shrink-input/run.ndjson"
        env["SHRINK_DIVERGENCE"] = "/shrink-input/div.json"
        env["SHRINK_MAX_STEP"] = str(shrink_max_step)
        env["SHRINK_VALIDATE_TIMEOUT"] = shrink_validate_timeout
        files["/shrink-input"] = shrink_artifact
    else:
        fail("unsupported mode '{}' (this launcher handles fuzz, shrink)".format(mode))

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
            files = files,
        ),
    )
    return service
