"""Shared RPC recipe helpers for xrpl-confluence tests.

Provides reusable PostHttpRequestRecipe builders and common wait patterns
to eliminate boilerplate across test files.
"""

# Well-known genesis account for private XRPL test networks (masterpassphrase).
GENESIS_ADDRESS = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
GENESIS_SECRET = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"

# Fixed test destination addresses (derived from known seeds for reproducibility).
TEST_DEST_1 = "rPMh7Pi9ct699iZUTWzJaUQx7cAS7KVpAt"
TEST_DEST_2 = "r3kmLJN5D28dHuH8vZNUZpMC43pEHpaocV"


def server_info_recipe():
    """Build a PostHttpRequestRecipe for server_info."""
    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{"method": "server_info", "params": [{}]}',
        extract = {
            "server_state": ".result.info.server_state",
            "validated_seq": ".result.info.validated_ledger.seq",
            "closed_seq": ".result.info.closed_ledger.seq",
            "peers": ".result.info.peers",
        },
    )


def ledger_recipe(ledger_index):
    """Build a PostHttpRequestRecipe for the ledger RPC method.

    Args:
        ledger_index: Integer ledger sequence number.

    Returns:
        A PostHttpRequestRecipe that extracts ledger_hash, account_hash, and
        transaction_hash from the response.
    """
    return PostHttpRequestRecipe(
        port_id = "rpc",
        endpoint = "/",
        content_type = "application/json",
        body = '{{"method": "ledger", "params": [{{"ledger_index": {}}}]}}'.format(ledger_index),
        extract = {
            "ledger_hash": ".result.ledger.ledger_hash",
            "account_hash": ".result.ledger.account_hash",
            "transaction_hash": ".result.ledger.transaction_hash",
            "close_time": ".result.ledger.close_time",
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


def wait_for_validated_seq(plan, node, min_seq, timeout = "120s"):
    """Wait until a node's validated ledger sequence reaches min_seq.

    Args:
        plan: Kurtosis plan object.
        node: Node descriptor dict with "name" key.
        min_seq: Minimum validated ledger sequence to wait for.
        timeout: Timeout string (default "120s").
    """
    plan.wait(
        service_name = node["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"seq": ".result.info.validated_ledger.seq"},
        ),
        field = "extract.seq",
        assertion = ">=",
        target_value = min_seq,
        timeout = timeout,
    )


def wait_for_closed_seq(plan, node, min_seq, timeout = "120s"):
    """Wait until a node's closed ledger sequence reaches min_seq.

    Uses closed_ledger.seq which is available before validated_ledger.
    Useful for liveness checks during early node startup.

    Args:
        plan: Kurtosis plan object.
        node: Node descriptor dict with "name" key.
        min_seq: Minimum closed ledger sequence to wait for.
        timeout: Timeout string (default "120s").
    """
    plan.wait(
        service_name = node["name"],
        recipe = PostHttpRequestRecipe(
            port_id = "rpc",
            endpoint = "/",
            content_type = "application/json",
            body = '{"method": "server_info", "params": [{}]}',
            extract = {"seq": ".result.info.closed_ledger.seq"},
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
