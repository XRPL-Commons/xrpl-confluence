"""Network topology and shared configuration generation."""

def generate_network_config(plan, rippled_count, goxrpl_count):
    """Generate shared network configuration for all nodes.

    Creates validator keys, peer lists, and genesis ledger config
    so that all nodes can form a single network.

    Args:
        plan: Kurtosis plan object.
        rippled_count: Number of rippled nodes.
        goxrpl_count: Number of goXRPL nodes.

    Returns:
        A files artifact containing configuration for all nodes.
    """
    total = rippled_count + goxrpl_count

    # Build peer list - each node needs to know about all other nodes
    peer_entries_rippled = []
    peer_entries_goxrpl = []
    validator_keys = []

    for i in range(rippled_count):
        peer_entries_rippled.append("rippled-{}".format(i))
    for i in range(goxrpl_count):
        peer_entries_goxrpl.append("goxrpl-{}".format(i))

    all_peers = peer_entries_rippled + peer_entries_goxrpl

    # Generate rippled config files
    config_files = {}
    for i in range(rippled_count):
        config_files["rippled-{}.cfg".format(i)] = _render_rippled_config(i, all_peers, total)

    # Generate goXRPL config files
    for i in range(goxrpl_count):
        config_files["goxrpl-{}.toml".format(i)] = _render_goxrpl_config(i, all_peers, total)

    return plan.render_templates(
        name = "network-config",
        config = {
            name: struct(template = content, data = {})
            for name, content in config_files.items()
        },
    )

def _render_rippled_config(index, all_peers, validator_count):
    """Render a rippled.cfg template for a given node index."""
    peers_section = ""
    for peer in all_peers:
        if peer != "rippled-{}".format(index):
            peers_section += "{{{{ .peer }}}} {}\n".format(51235)

    # TODO: flesh out with real rippled config template
    return """
[server]
port_peer
port_rpc
port_ws

[port_peer]
port = 51235
ip = 0.0.0.0
protocol = peer

[port_rpc]
port = 5005
ip = 0.0.0.0
admin = 127.0.0.1
protocol = http

[port_ws]
port = 6006
ip = 0.0.0.0
protocol = ws

[node_db]
type = NuDB
path = /var/lib/rippled/db/nudb
advisory_delete = 0
online_delete = 256

[ledger_history]
256

[node_size]
tiny

[ips_fixed]
{peers}

[validators_file]
validators.txt

[rpc_startup]
{{ "command": "log_level", "severity": "warning" }}

[sntp_servers]
time.windows.com
time.apple.com
time.nist.gov
pool.ntp.org
""".format(peers = peers_section)

def _render_goxrpl_config(index, all_peers, validator_count):
    """Render a goXRPL config template for a given node index."""
    peers_list = ""
    for peer in all_peers:
        if peer != "goxrpl-{}".format(index):
            peers_list += '  "{}:{}",\n'.format(peer, 51235)

    # TODO: flesh out with real goXRPL config template
    return """
[server]
peer_port = 51235
rpc_port = 5005
ws_port = 6006

[node]
size = "tiny"
db_path = "/var/lib/goxrpl/db"

[peers]
fixed = [
{peers}]

[consensus]
validator = true
""".format(peers = peers_list)
