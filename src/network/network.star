"""Runtime helpers for long-lived local rippled networks."""

READINESS_SCRIPT = read_file(src = "./readiness.py")


def wait_for_readiness(plan, nodes, timeout_seconds = 180):
    """Run the bounded common-validated-ledger check for all node RPCs."""
    if len(nodes) < 2:
        fail("network readiness requires at least two rippled nodes")
    if timeout_seconds <= 0:
        fail("network_ready_timeout_seconds must be positive (got {})".format(timeout_seconds))

    urls = []
    for node in nodes:
        urls.append(node["rpc_url"])

    # Give the task a small amount of cleanup/reporting time beyond the
    # application deadline.  run_python's default acceptable code [0] makes a
    # readiness failure fail the Kurtosis run with the script diagnostics.
    result = plan.run_python(
        run = READINESS_SCRIPT,
        args = [
            "--timeout-seconds",
            str(timeout_seconds),
            "--poll-interval-seconds",
            "2",
        ] + urls,
        image = "python:3.11-alpine",
        wait = "{}s".format(timeout_seconds + 30),
        description = "waiting for all rippled nodes to share a validated ledger",
    )
    plan.print(result.output)
    return result
