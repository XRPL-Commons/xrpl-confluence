"""Network topology and shared configuration generation.

Generates valid rippled (.cfg) and goXRPL (.toml) configs for a private
test network, including shared validator keys and peer lists.
"""

# Pre-generated validator keypairs (secp256k1).
# Generated via scripts/keygen/main.go using goXRPL crypto.
# Each node gets one keypair; the seed goes into that node's config,
# and ALL public keys go into every node's validators file.
VALIDATOR_KEYS = [
    {"seed": "sneWFZcEqA8TUA5BmJ38xsqaR7dFb", "pubkey": "n9LXMXFTeVL6o9fxdFHfeVZWf6YzWCBzt7YyeK1HV7wZ4ZFRNgUV"},
    {"seed": "snjbY5o3g4zK8dtotD6wjdNV3i96r", "pubkey": "n9KTo9UAFTV2XPZG8oUbuwNBhvwVF2fkyxz9jE88iGhJVoV3Sxy4"},
    {"seed": "sn8KuG4fs84rowCsqTuz6AtqEkmJ7", "pubkey": "n9KVs96MmgjXmok33PNEr29xbRAfvqvw1HqQYGsWE9zBdJMYJ9Pc"},
    {"seed": "sha6zPXQHAEwVk1qEREAxZPqy7h5Z", "pubkey": "n9KRLEqrFzXi5yK3XE6NUhcFx8XLHWZg3SczPb8doFCiryPSmvfr"},
    {"seed": "snPRr5dyXnYYZ4idydxHxhm2qnohc", "pubkey": "n9Jjt6fFpdTzms5tpYAf2iFyQwXNZWrQgwtrbwQEvFWQN4kfRFPb"},
]

NETWORK_ID = 10000
PEER_PORT = 51235
RPC_PORT = 5005
WS_PORT = 6006


def generate_network_config(plan, rippled_count, goxrpl_count):
    """Generate shared network configuration for all nodes.

    Creates per-node config files with validator keys, peer lists, and
    shared validator trust files so all nodes form a single private network.

    Args:
        plan: Kurtosis plan object.
        rippled_count: Number of rippled nodes.
        goxrpl_count: Number of goXRPL nodes.

    Returns:
        A files artifact containing configuration for all nodes.
    """
    total = rippled_count + goxrpl_count
    if total > len(VALIDATOR_KEYS):
        fail("Requested {} nodes but only {} validator keys are available".format(total, len(VALIDATOR_KEYS)))

    # Build service name lists
    rippled_names = ["rippled-{}".format(i) for i in range(rippled_count)]
    goxrpl_names = ["goxrpl-{}".format(i) for i in range(goxrpl_count)]
    all_names = rippled_names + goxrpl_names

    # Collect all validator public keys
    all_pubkeys = [VALIDATOR_KEYS[i]["pubkey"] for i in range(total)]

    config_files = {}

    # Per-node rippled configs
    for i in range(rippled_count):
        peers = [name for name in all_names if name != rippled_names[i]]
        config_files["rippled-{}.cfg".format(i)] = _render_rippled_config(
            index = i,
            node_key = VALIDATOR_KEYS[i],
            peers = peers,
        )

    # Per-node goXRPL configs
    for i in range(goxrpl_count):
        key_index = rippled_count + i
        peers = [name for name in all_names if name != goxrpl_names[i]]
        config_files["goxrpl-{}.toml".format(i)] = _render_goxrpl_config(
            index = i,
            node_key = VALIDATOR_KEYS[key_index],
            peers = peers,
        )

    # Shared validators files
    config_files["validators.txt"] = _render_validators_txt(all_pubkeys)
    config_files["validators.toml"] = _render_validators_toml(all_pubkeys)

    return plan.render_templates(
        name = "network-config",
        config = {
            name: struct(template = content, data = {})
            for name, content in config_files.items()
        },
    )


def _render_rippled_config(index, node_key, peers):
    """Render a complete rippled.cfg for a private test network node."""
    peers_section = ""
    for peer in peers:
        peers_section += "{} {}\n".format(peer, PEER_PORT)

    return """\
[server]
port_peer
port_rpc
port_ws

[port_peer]
port={peer_port}
ip=0.0.0.0
protocol=peer

[port_rpc]
port={rpc_port}
ip=0.0.0.0
admin=0.0.0.0
protocol=http

[port_ws]
port={ws_port}
ip=0.0.0.0
admin=0.0.0.0
protocol=ws

[node_db]
type=NuDB
path=/var/lib/rippled/db/nudb
online_delete=256
advisory_delete=0

[database_path]
/var/lib/rippled/db

[debug_logfile]
/var/lib/rippled/db/debug.log

[node_size]
tiny

[ips_fixed]
{peers}

[peer_private]
1

[network_id]
{network_id}

[validation_seed]
{seed}

[validators_file]
validators.txt

[sntp_servers]
time.windows.com
time.apple.com
time.nist.gov
pool.ntp.org

[rpc_startup]
{{"command": "log_level", "severity": "warning"}}

[ledger_history]
256

[ssl_verify]
0
""".format(
        peer_port = PEER_PORT,
        rpc_port = RPC_PORT,
        ws_port = WS_PORT,
        peers = peers_section,
        network_id = NETWORK_ID,
        seed = node_key["seed"],
    )


def _render_goxrpl_config(index, node_key, peers):
    """Render a complete goXRPL xrpld.toml for a private test network node."""
    ips_fixed_entries = ""
    for peer in peers:
        ips_fixed_entries += '    "{} {}",\n'.format(peer, PEER_PORT)

    return """\
compression = false
peer_private = 1
peers_max = 50
max_transactions = 250
ips = []
ips_fixed = [
{ips_fixed}]

relay_proposals = "trusted"
relay_validations = "all"
ledger_history = 256
fetch_depth = "full"

path_search = 2
path_search_fast = 2
path_search_max = 3
path_search_old = 2

workers = 0
io_workers = 0
prefetch_workers = 0

network_id = {network_id}
ledger_replay = 0
ssl_verify = 0

database_path = "/tmp/goxrpl/db"
debug_logfile = "/tmp/goxrpl/db/debug.log"

node_size = "tiny"
signing_support = false
beta_rpc_api = 0

validation_seed = "{seed}"
validators_file = "/etc/goxrpl/validators.toml"

rpc_startup = [
    {{ command = "log_level", severity = "warning" }}
]

[logging]
level = "info"
format = "text"
output = "stdout"

[server]
ports = ["port_rpc_admin_local", "port_ws_admin_local", "port_peer"]

[port_rpc_admin_local]
port = {rpc_port}
ip = "0.0.0.0"
admin = ["0.0.0.0"]
protocol = "http"

[port_ws_admin_local]
port = {ws_port}
ip = "0.0.0.0"
admin = ["0.0.0.0"]
protocol = "ws"

[port_peer]
port = {peer_port}
ip = "0.0.0.0"
protocol = "peer"

[node_db]
type = "pebble"
path = "/tmp/goxrpl/db/pebble"
online_delete = 256
advisory_delete = 0
cache_size = 16384
cache_age = 5
fast_load = false
earliest_seq = 0
delete_batch = 100
back_off_milliseconds = 100
age_threshold_seconds = 60
recovery_wait_seconds = 5

[sqlite]
journal_mode = "wal"
synchronous = "normal"
temp_store = "file"
page_size = 4096
journal_size_limit = 1582080

[overlay]
max_unknown_time = 600
max_diverged_time = 300

[transaction_queue]
ledgers_in_queue = 20
minimum_queue_size = 2000
retry_sequence_percent = 25
minimum_escalation_multiplier = 500
minimum_txn_in_ledger = 5
minimum_txn_in_ledger_standalone = 1000
target_txn_in_ledger = 50
maximum_txn_in_ledger = 0
normal_consensus_increase_percent = 20
slow_consensus_decrease_percent = 50
maximum_txn_per_account = 10
minimum_last_ledger_buffer = 2
zero_basefee_transaction_feelevel = 256000
""".format(
        ips_fixed = ips_fixed_entries,
        network_id = NETWORK_ID,
        seed = node_key["seed"],
        rpc_port = RPC_PORT,
        ws_port = WS_PORT,
        peer_port = PEER_PORT,
    )


def _render_validators_txt(pubkeys):
    """Render a validators.txt file for rippled (INI format)."""
    lines = "[validators]\n"
    for key in pubkeys:
        lines += "{}\n".format(key)
    return lines


def _render_validators_toml(pubkeys):
    """Render a validators.toml file for goXRPL (TOML format)."""
    entries = ""
    for i, key in enumerate(pubkeys):
        comma = "," if i < len(pubkeys) - 1 else ""
        entries += '    "{}"{}\n'.format(key, comma)

    return """\
validators = [
{entries}]
validator_list_sites = []
validator_list_keys = []
""".format(entries = entries)
