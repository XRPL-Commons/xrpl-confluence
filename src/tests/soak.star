"""Soak test suite.

Launches the fuzz sidecar in unbounded MODE=soak against the mixed network.
The sidecar runs indefinitely — the enclave must be torn down externally
(e.g. via `make soak-down` or `kurtosis enclave rm`).

Corpus output is written to the Kurtosis persistent volume "fuzz-soak-output"
mounted at /output/corpus inside the fuzz-soak container. To extract it
before teardown:

    kurtosis service exec <enclave> fuzz-soak -- \
        tar -C /output -czf - corpus | tar -C /tmp/corpus -xzf -

See C5 `make soak-pull` for the automated extraction workflow.
"""

helpers = import_module("../helpers/rpc.star")
fuzz_sidecar = import_module("../sidecar/fuzz.star")


def run(plan, nodes, args = {}):
    """Run the soak suite (unbounded).

    Args:
        plan: Kurtosis plan object.
        nodes: List of all node descriptors (rippled + goXRPL).
        args: Optional configuration dict.
            - tx_rate: Transactions per second (0 = unlimited, default 0).
            - rotate_every: Rotate account tier every N txs (default 1000).
            - mutation_rate: Fraction [0,1] of txs to mutate (default 0.0).
            - accounts: Number of test accounts (default 50).
            - corpus_host_path: Desired host path for corpus bind-mount
              (currently ignored — Kurtosis 1.x lacks host_path support;
              corpus goes into persistent volume "fuzz-soak-output").

    Returns:
        Dict with "fuzz-soak" key once the service is up.
    """
    tx_rate = args.get("tx_rate", 0)
    rotate_every = args.get("rotate_every", 1000)
    mutation_rate = args.get("mutation_rate", 0.0)
    accounts = args.get("accounts", 50)
    corpus_host_path = args.get("corpus_host_path", "")

    # Soak runs are about exercising goXRPL hard against multiple rippled
    # validators. Require at least 2 rippled (so quorum survives one going
    # down) and at least 1 goXRPL (otherwise this is just rippled testing
    # itself). The Makefile defaults to 3+2 — see top-level Makefile.
    rippled_nodes_count = len([n for n in nodes if n["type"] == "rippled"])
    goxrpl_nodes_count = len([n for n in nodes if n["type"] == "goxrpl"])
    if rippled_nodes_count < 2 or goxrpl_nodes_count < 1:
        fail("soak suite requires >= 2 rippled and >= 1 goXRPL (got {} rippled, {} goxrpl)".format(
            rippled_nodes_count, goxrpl_nodes_count,
        ))

    plan.print("Waiting for all nodes to reach closed_seq >= 3...")
    for node in nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0] if len(rippled_nodes) > 0 else nodes[0]

    plan.print("Launching fuzz-soak sidecar (tx_rate={}, accounts={}, rotate_every={}, mutation_rate={})...".format(
        tx_rate, accounts, rotate_every, mutation_rate,
    ))

    svc = fuzz_sidecar.launch_soak(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        tx_rate = tx_rate,
        rotate_every = rotate_every,
        mutation_rate = mutation_rate,
        accounts = accounts,
        corpus_host_path = corpus_host_path,
    )

    plan.print("fuzz-soak service is up. Corpus accumulates in persistent volume 'fuzz-soak-output' at /output/corpus.")
    plan.print("Leave the enclave running; tear down with `kurtosis enclave rm <enclave>` or `make soak-down`.")

    if args.get("enable_observability", False):
        prom = import_module("../sidecar/prometheus.star")
        graf = import_module("../sidecar/grafana.star")
        prom.launch(plan, fuzz_service_name = "fuzz-soak")
        graf.launch(plan, prometheus_service_name = "prometheus")
        plan.print("observability: prometheus on :9090, grafana on :3000 (anonymous viewer)")

    return {"fuzz-soak": svc}
