"""Chaos test suite — soak loop + scheduled disturbances.

The chaos sidecar runs indefinitely. The schedule is supplied via
args["chaos_args"]["schedule"] as a JSON string (see
sidecar/internal/fuzz/chaos/schedule_parse.go for the wire format).
Tear down with `kurtosis enclave rm <name>` or `make chaos-down`.
"""

helpers = import_module("../helpers/rpc.star")
fuzz_sidecar = import_module("../sidecar/fuzz.star")


def run(plan, nodes, args = {}):
    """Run the chaos suite (unbounded).

    Args (under args):
        - schedule: JSON string. Required.
        - tx_rate, rotate_every, mutation_rate, accounts: same as soak.
    """
    schedule = args.get("schedule", "")
    if schedule == "":
        fail("chaos suite requires args.schedule (JSON array)")

    tx_rate = args.get("tx_rate", 0)
    rotate_every = args.get("rotate_every", 1000)
    mutation_rate = args.get("mutation_rate", 0.0)
    accounts = args.get("accounts", 50)

    rippled_nodes_count = len([n for n in nodes if n["type"] == "rippled"])
    if rippled_nodes_count < 2:
        fail("chaos suite requires >= 2 rippled (got {})".format(rippled_nodes_count))

    rippled_nodes = [n for n in nodes if n["type"] == "rippled"]
    submit_node = rippled_nodes[0]

    # Only wait on rippled nodes. With the rippled-only UNL (see topology.star)
    # goXRPL can lag far behind without blocking quorum, and at the time of
    # writing goXRPL has a passive-consensus bug that keeps it at genesis until
    # validations from rippled reach it correctly. Gating chaos launch on
    # goXRPL would deadlock on that bug. The chaos runner itself submits txs
    # through the rippled submit node and oracle-checks the goXRPL nodes, so
    # any goXRPL divergence still surfaces — it just no longer blocks startup.
    plan.print("Waiting for rippled nodes to reach closed_seq >= 3...")
    for node in rippled_nodes:
        helpers.wait_for_ledger_seq(plan, node, 3, timeout = "120s")

    plan.print("Launching fuzz-chaos sidecar")

    svc = fuzz_sidecar.launch_chaos(
        plan,
        all_nodes = nodes,
        submit_node = submit_node,
        chaos_schedule = schedule,
        tx_rate = tx_rate,
        rotate_every = rotate_every,
        mutation_rate = mutation_rate,
        accounts = accounts,
    )
    return {"fuzz-chaos": svc}
