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
    image = "xrpl-confluence-sidecar:latest",
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
        image: Docker image containing /fuzz binary (default "xrpl-confluence-sidecar:latest").
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
        "MODE":             mode,
        "NODES":            node_urls,
        "SUBMIT_URL":       submit_url,
        "ACCOUNTS":         str(accounts),
        "BATCH_CLOSE":      batch_close,
        "CORPUS_DIR":       "/output/corpus",
        "CRASH_LABEL_KEY":  "com.kurtosistech.custom.fuzzer.role",
        "CRASH_LABEL_VAL":  "node",
        "CRASH_TAIL_LINES": "200",
        "DOCKER_HOST":      "tcp://host.docker.internal:2375",
    }
    # NOTE: Kurtosis 1.x has no Starlark primitive for host-path bind mounts,
    # so the Docker socket cannot be injected via this launcher alone. Crash
    # detection requires one of:
    #   - On Linux: run the Kurtosis engine with --volume
    #     /var/run/docker.sock:/var/run/docker.sock and have the engine
    #     forward that mount into enclaves.
    #   - Run a docker-socket-proxy sidecar (e.g. tecnativa/docker-socket-proxy)
    #     exposing TCP 2375 in the enclave, and set DOCKER_HOST=tcp://proxy:2375
    #     in this env block.
    # Without one of those, NewDockerRuntime() Pings and fails fast; the runner
    # then logs "crash poller disabled — docker dial failed" and continues
    # without crash detection. The fuzz/oracle layers still work.
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


def launch_soak(
    plan,
    all_nodes,
    submit_node,
    tx_rate = 0,
    rotate_every = 1000,
    mutation_rate = 0.0,
    accounts = 50,
    corpus_host_path = ""):
    """Launch the fuzz sidecar in soak (unbounded) mode.

    See launch() for fuzz/shrink modes; this wrapper keeps soak's longer-lived
    distinct service name (fuzz-soak) and persistent-volume output mount.

    NOTE: Kurtosis 1.x does not support Directory(host_path=...) — the argument
    is rejected at interpretation time with "unexpected keyword argument host_path".
    The corpus is therefore stored in a Kurtosis persistent volume keyed
    "fuzz-soak-output". After the enclave is torn down the volume persists and
    can be extracted via `kurtosis service exec fuzz-soak 'tar -C /output -czf - corpus'`
    before teardown, or via the C5 `make soak-pull` target which copies it out
    with `docker cp`. The corpus_host_path argument is accepted for forward
    compatibility but is currently ignored.

    Args:
        plan: Kurtosis plan object.
        all_nodes: List of all node descriptors.
        submit_node: Node descriptor to submit transactions to.
        tx_rate: Transactions per second (0 = unlimited).
        rotate_every: Rotate account tier every N transactions.
        mutation_rate: Fraction [0,1] of transactions to mutate.
        accounts: Number of test accounts to create.
        corpus_host_path: Desired host path for corpus bind-mount (currently
            ignored — see NOTE above). Reserved for when Kurtosis adds support.

    Returns:
        The fuzz-soak service reference.
    """
    node_urls = ",".join([
        "http://{}:5005".format(n["name"]) for n in all_nodes
    ])
    submit_url = "http://{}:5005".format(submit_node["name"])

    # Kurtosis 1.x has no host-path bind-mount primitive. Use a persistent
    # volume so the corpus survives enclave restarts within the same run.
    files = {
        "/output": Directory(persistent_key = "fuzz-soak-output"),
    }

    return plan.add_service(
        name = "fuzz-soak",
        config = ServiceConfig(
            image = "xrpl-confluence-sidecar:latest",
            entrypoint = ["/fuzz"],
            ports = {
                "results": PortSpec(
                    number = 8081,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = files,
            env_vars = {
                "MODE":             "soak",
                "NODES":            node_urls,
                "SUBMIT_URL":       submit_url,
                "ACCOUNTS":         str(accounts),
                "TX_RATE":          str(tx_rate),
                "ROTATE_EVERY":     str(rotate_every),
                "MUTATION_RATE":    str(mutation_rate),
                "CORPUS_DIR":       "/output/corpus",
                "CRASH_LABEL_KEY":  "com.kurtosistech.custom.fuzzer.role",
                "CRASH_LABEL_VAL":  "node",
                "CRASH_TAIL_LINES": "200",
                "DOCKER_HOST":      "tcp://host.docker.internal:2375",
            },
        ),
    )


def launch_chaos(
    plan,
    all_nodes,
    submit_node,
    chaos_schedule,
    tx_rate = 0,
    rotate_every = 1000,
    mutation_rate = 0.0,
    accounts = 50):
    """Launch the fuzz sidecar in chaos mode.

    Same wiring as launch_soak plus CHAOS_SCHEDULE (JSON string). The
    sidecar mounts the persistent volume `fuzz-chaos-output` at
    /output. Crash detection requires the host to forward
    /var/run/docker.sock into the enclave (Linux) or run a
    docker-socket-proxy (macOS); without that the chaos events
    NetworkRuntime construction fails fast and the runner logs
    "chaos: NetworkRuntime disabled".
    """
    node_urls = ",".join(["http://{}:5005".format(n["name"]) for n in all_nodes])
    submit_url = "http://{}:5005".format(submit_node["name"])

    files = {
        "/output": Directory(persistent_key = "fuzz-chaos-output"),
    }

    return plan.add_service(
        name = "fuzz-chaos",
        config = ServiceConfig(
            image = "xrpl-confluence-sidecar:latest",
            entrypoint = ["/fuzz"],
            ports = {
                "results": PortSpec(
                    number = 8081,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = files,
            env_vars = {
                "MODE":             "chaos",
                "NODES":            node_urls,
                "SUBMIT_URL":       submit_url,
                "ACCOUNTS":         str(accounts),
                "TX_RATE":          str(tx_rate),
                "ROTATE_EVERY":     str(rotate_every),
                "MUTATION_RATE":    str(mutation_rate),
                "CORPUS_DIR":       "/output/corpus",
                "CRASH_LABEL_KEY":  "com.kurtosistech.custom.fuzzer.role",
                "CRASH_LABEL_VAL":  "node",
                "CRASH_TAIL_LINES": "200",
                "DOCKER_HOST":      "tcp://host.docker.internal:2375",
                "CHAOS_SCHEDULE":   chaos_schedule,
            },
        ),
    )
