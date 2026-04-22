"""Shared RPC recipe helpers for xrpl-confluence tests.

Provides reusable PostHttpRequestRecipe builders and common wait patterns
to eliminate boilerplate across test files.

Note: goXRPL nodes currently operate in "full" mode (syncing) rather than
"proposing" mode, so `validated_ledger` may not exist in server_info.
All wait functions use `closed_ledger.seq` which is always present.
For hash comparison, we use the `ledger` RPC with a specific index which
works regardless of validation state.
"""

# Well-known genesis account for private XRPL test networks (masterpassphrase).
GENESIS_ADDRESS = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
GENESIS_SECRET = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"

# Fixed test destination addresses (generated with valid checksums via goXRPL crypto).
TEST_DEST_1 = "ra8sezk7XT7JRgE1myhUBZJUDCUH3qrWMU"
TEST_DEST_2 = "rDimGkRfnAufCwoKbrAXSYDCcjgth548s2"


def server_info_recipe():
    """Build a PostHttpRequestRecipe for server_info.

    Only extracts fields that are guaranteed to be present in all node states.
    Note: closed_ledger and validated_ledger are mutually exclusive in rippled
    (depends on whether quorum is met), so neither is extracted here.
    The full response body is logged by plan.request() regardless.
    """
    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{"method": "server_info", "params": [{}]}',
        extract = {
            "server_state": ".result.info.server_state",
            "peers": ".result.info.peers",
        },
    )


def ledger_recipe(ledger_index):
    """Build a PostHttpRequestRecipe for the ledger RPC method.

    Args:
        ledger_index: Integer ledger sequence number OR one of the
            XRPL string selectors "validated", "closed", "current".
            Passing a string (e.g. "validated") is the most robust
            choice in tests because it avoids races where a hardcoded
            seq hasn't been closed/validated on the target node yet.

    Returns:
        A PostHttpRequestRecipe that extracts ledger_hash, account_hash, and
        transaction_hash from the response.
    """

    # Quote string selectors like "validated"; leave integer seqs bare.
    if type(ledger_index) == "string":
        index_literal = '"{}"'.format(ledger_index)
    else:
        index_literal = "{}".format(ledger_index)

    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{{"method": "ledger", "params": [{{"ledger_index": {}}}]}}'.format(index_literal),
        extract = {
            "ledger_hash": ".result.ledger.ledger_hash",
            "account_hash": ".result.ledger.account_hash",
            "transaction_hash": ".result.ledger.transaction_hash",
            "close_time": ".result.ledger.close_time",
            "ledger_index": ".result.ledger.ledger_index",
        },
    )


def account_info_recipe(account):
    """Build a PostHttpRequestRecipe for account_info.

    Args:
        account: XRPL account address string.

    Returns:
        A PostHttpRequestRecipe that extracts balance and sequence.
    """
    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{{"method": "account_info", "params": [{{"account": "{}"}}]}}'.format(account),
        extract = {
            "balance": ".result.account_data.Balance",
            "sequence": ".result.account_data.Sequence",
            "status": ".result.status",
        },
    )


def submit_payment_recipe(secret, account, destination, amount):
    """Build a sign-and-submit Payment recipe.

    Uses rippled's admin RPC to auto-fill Sequence, Fee, and LastLedgerSequence.

    Args:
        secret: Signing secret (base58 seed).
        account: Source account address.
        destination: Destination account address.
        amount: Amount in drops (string or int).

    Returns:
        A PostHttpRequestRecipe that extracts the engine result and tx hash.
    """
    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{{"method": "submit", "params": [{{"secret": "{secret}", "tx_json": {{"TransactionType": "Payment", "Account": "{account}", "Destination": "{dest}", "Amount": "{amount}"}}}}]}}'.format(
            secret = secret,
            account = account,
            dest = destination,
            amount = amount,
        ),
        extract = {
            "engine_result": ".result.engine_result",
            "tx_hash": ".result.tx_json.hash",
            "status": ".result.status",
        },
    )


def wait_for_ledger_seq(plan, node, min_seq, timeout = "120s"):
    """Wait until a node's current ledger index reaches min_seq.

    Uses the ledger_current RPC which returns ledger_current_index — a field
    that is always present regardless of whether the network has reached
    validation quorum (server_info's closed_ledger and validated_ledger fields
    are mutually exclusive depending on node state).

    Args:
        plan: Kurtosis plan object.
        node: Node descriptor dict with "name" key.
        min_seq: Minimum ledger sequence to wait for.
        timeout: Timeout string (default "120s").
    """
    plan.wait(
        service_name = node["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "ledger_current", "params": [{}]}',
            extract = {"seq": ".result.ledger_current_index"},
        ),
        field = "extract.seq",
        assertion = ">=",
        target_value = min_seq,
        timeout = timeout,
    )


def wait_for_peers(plan, node, min_peers, timeout = "60s"):
    """Wait until a node has at least min_peers connected peers.

    Args:
        plan: Kurtosis plan object.
        node: Node descriptor dict with "name" key.
        min_peers: Minimum number of peers.
        timeout: Timeout string (default "60s").
    """
    plan.wait(
        service_name = node["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"peers": ".result.info.peers"},
        ),
        field = "extract.peers",
        assertion = ">=",
        target_value = min_peers,
        timeout = timeout,
    )


def query_all_server_info(plan, nodes):
    """Query and print server_info for every node.

    Args:
        plan: Kurtosis plan object.
        nodes: List of node descriptor dicts.
    """
    for node in nodes:
        plan.print("  server_info for " + node["name"] + " (" + node["type"] + ")")
        plan.request(
            service_name = node["name"],
            recipe = server_info_recipe(),
        )


def query_ledger_hashes(plan, nodes, seq):
    """Query and print ledger hashes for a specific sequence on every node.

    Since Starlark cannot compare extracted values across nodes, this function
    logs all hashes prominently for visual inspection or CI log parsing.

    Args:
        plan: Kurtosis plan object.
        nodes: List of node descriptor dicts.
        seq: Ledger sequence number to query.
    """
    plan.print("=== Ledger hash comparison for seq {} ===".format(seq))
    for node in nodes:
        plan.print("  Querying ledger {} on {} ({})".format(seq, node["name"], node["type"]))
        plan.request(
            service_name = node["name"],
            recipe = ledger_recipe(seq),
        )
    plan.print("=== End ledger hash comparison ===")


def assert_validated_ledgers_match(plan, nodes):
    """Assert every node reports the same validated ledger hash.

    Uses ledger_index="validated" so each node returns whatever its
    latest validated ledger is — no hardcoded seqs, no race against
    the close/validate cycle. The first node's validated hash is
    captured and every subsequent node's validated hash is compared
    to it via plan.verify, which supports runtime-value substitution
    across service boundaries.

    This does NOT retry — it's a point-in-time snapshot. Callers are
    expected to have already driven the follower node to a synced
    state (via wait_for_ledger_seq) before invoking this. If a
    follower hasn't converged yet, the verify fails loudly rather
    than silently waiting.

    Args:
        plan: Kurtosis plan object.
        nodes: List of node descriptor dicts (length >= 2). The first
            node is the reference; all others must match it.
    """
    if len(nodes) < 2:
        fail("assert_validated_ledgers_match needs at least two nodes")

    reference = nodes[0]
    plan.print(
        "=== Validated-ledger hash match vs {} ===".format(reference["name"]),
    )

    # Capture the reference node's validated ledger hash.
    ref_result = plan.request(
        service_name = reference["name"],
        recipe = ledger_recipe("validated"),
        acceptable_codes = [200],
        description = "capture reference validated ledger",
    )

    # For every follower, query its validated hash and verify equality
    # against the reference. plan.verify does runtime substitution on
    # both sides of the comparison, so cross-service hash matching
    # actually works (unlike plan.wait's target_value, which is
    # evaluated as a literal string).
    for node in nodes[1:]:
        plan.print(
            "  {} validated ledger must match {}".format(
                node["name"],
                reference["name"],
            ),
        )
        follower_result = plan.request(
            service_name = node["name"],
            recipe = ledger_recipe("validated"),
            acceptable_codes = [200],
            description = "capture {} validated ledger".format(node["name"]),
        )
        plan.verify(
            value = follower_result["extract.ledger_hash"],
            assertion = "==",
            target_value = ref_result["extract.ledger_hash"],
        )
    plan.print("=== Validated-ledger hash match confirmed ===")
