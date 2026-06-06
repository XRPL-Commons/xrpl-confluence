"""Network topology and shared configuration generation.

Generates valid rippled (.cfg) and go-xrpl (.toml) configs for a private
test network, including shared validator keys and peer lists.
"""

# Pre-generated validator keypairs (secp256k1).
# Generated via scripts/keygen/main.go using go-xrpl crypto.
# Each node gets one keypair; the seed goes into that node's config,
# and ALL public keys go into every node's validators file.
VALIDATOR_KEYS = [
    {"seed": "sneWFZcEqA8TUA5BmJ38xsqaR7dFb", "pubkey": "n9LXMXFTeVL6o9fxdFHfeVZWf6YzWCBzt7YyeK1HV7wZ4ZFRNgUV"},
    {"seed": "snjbY5o3g4zK8dtotD6wjdNV3i96r", "pubkey": "n9KTo9UAFTV2XPZG8oUbuwNBhvwVF2fkyxz9jE88iGhJVoV3Sxy4"},
    {"seed": "sn8KuG4fs84rowCsqTuz6AtqEkmJ7", "pubkey": "n9KVs96MmgjXmok33PNEr29xbRAfvqvw1HqQYGsWE9zBdJMYJ9Pc"},
    {"seed": "sha6zPXQHAEwVk1qEREAxZPqy7h5Z", "pubkey": "n9KRLEqrFzXi5yK3XE6NUhcFx8XLHWZg3SczPb8doFCiryPSmvfr"},
    {"seed": "snPRr5dyXnYYZ4idydxHxhm2qnohc", "pubkey": "n9Jjt6fFpdTzms5tpYAf2iFyQwXNZWrQgwtrbwQEvFWQN4kfRFPb"},
    {"seed": "saa7XDheiBUhj8uM57KW2VjBFNZ5C", "pubkey": "n94EazZJELsns1Kizfj88S9vxbaEkrMQzmxayzCnFnN3EpDUGUdK"},
    {"seed": "ssjs6bcRJMiZEDMMBYUAZTbQfvvmk", "pubkey": "n9Kkh8kM1Hq79u3mYevtf5tA3yis5YEnzjJtqqBiDEaGwgYnjjcA"},
    {"seed": "ssYZgL9h161e6d8LYU1huZiZ566hd", "pubkey": "n9KbfoZv1Br87TBbdNt5JJXidcYnRmZYVNbsrscB2taETRKxtcMw"},
    {"seed": "spq3gv3ohNqg24wjaiCU4LG9CHUKV", "pubkey": "n9Ka23g3zJt52d6SVJAWAYyQqDM3r4fJ5AmFHHxdgsx5UHa9enL5"},
    {"seed": "ssdUAKmoDAKzPmRGZs9E4VKyLYZb6", "pubkey": "n9KfqDFGG3QQdnUyvbwiUZ3vhx8FCrXfA5WEo2xqXD3TdvKX8i6g"},
]

NETWORK_ID = 10000
PEER_PORT = 51235
RPC_PORT = 5005
WS_PORT = 6006

# Full advanced amendment set enabled at GENESIS on BOTH rippled and go-xrpl so
# the two implementations build a byte-identical genesis ledger. This is the
# go-xrpl SupportedYes set MINUS XRPFees (changes the FeeSettings genesis format)
# and MINUS every obsolete/retired amendment (rippled's getDesired() at --start
# can never vote an obsolete amendment up, so including them would fork genesis).
# IDs are sha512-half(name), taken from go-xrpl's amendment registry, sorted
# ascending by ID to match rippled's sorted STVector256 in the Amendments SLE.
GENESIS_AMENDMENTS = [
    ("00C1FC4A53E60AB02C864641002B3172F38677E29C26C5406685179B37E1EDAC", "RequireFullyCanonicalSig"),
    ("03BDC0099C4E14163ADA272C1B6F6FABB448CC3E51F522F978041E4B57D9158C", "fixNFTokenReserve"),
    ("12523DF04B553A0B1AD74F42DDB741DE8DC06A03FC089A0EF197E2A87F1D8107", "fixAMMOverflowOffer"),
    ("138B968F25822EFBF54C00F97031221C47B1EAB8321D93C7C2AEAF85F04EC5DF", "TokenEscrow"),
    ("157D2D480E006395B76F948E3E07A45A05FE10230D88A7993C71F97AE4B1F2D1", "Checks"),
    ("15D61F0C6DB6A2F86BCF96F1E2444FEC54E705923339EC175BD3E517C8B3FF91", "fixDisallowIncomingV1"),
    ("1CB67D082CF7D9102412D34258CEDB400E659352D3B207348889297A6D90F5EF", "Credentials"),
    ("1E7ED950F2F13C4F8E2A54103B74D57D5D298FFDBD005936164EE9E6484C438C", "fixAMMv1_2"),
    ("1F4AFA8FA1BC8827AD4C0F682C03A8B671DCDF6B5C4DE36D44243A684103EF88", "HardenedValidations"),
    ("25BA44241B3BD880770BFA4DA21C7180576831855368CBEC6A3154FDE4A7676E", "fix1781"),
    ("27CD95EE8E1E5A537FF2F89B6CEB7C622E78E9374EBD7DCBEDFAE21CD6F16E0A", "fixReducedOffersV1"),
    ("2BF037D90E1B676B17592A8AF55E88DB465398B4B597AE46EECEE1399AB05699", "fixXChainRewardRounding"),
    ("2CD5286D8D687E98B41102BDD797198E81EA41DF7BD104E6561FEB104EFF2561", "fixTakerDryOfferRemoval"),
    ("2E2FB9CF8A44EB80F4694D38AADAE9B8B7ADAFD2F092E10068E61C98C4F092B0", "fixUniversalNumber"),
    ("30CD365592B8EE40489BA01AE2F7555CAC9C983145871DC82A42A31CF5BAE7D9", "DeletableAccounts"),
    ("31E0DA76FB8FB527CADCDF0E61CB9C94120966328EFA9DCA202135BAF319C0BA", "fixReducedOffersV2"),
    ("32A122F1352A4C7B3A6D790362CC34749C5E57FCE896377BFDC6CCD14F6CD627", "NonFungibleTokensV1_1"),
    ("3318EA0CF0755AF15DAC19F2B5C5BCBFF4B78BDD57609ACCAABE2C41309B051A", "fixFillOrKill"),
    ("35291ADD2D79EB6991343BDA0912269C817D0F094B02226C1C14AD2858962ED4", "fixAMMv1_1"),
    ("3CBC5C4E630A1B82380295CDA84B32B49DD066602E74E39B85EF64137FA65194", "DepositPreauth"),
    ("41765F664A8D67FF03DDB1C1A893DE6273690BA340A6C2B07C8D29D0DD013D3A", "fixDirectoryLimit"),
    ("452F5906C46D46F407883344BFDD90E672B672C5E9943DB4891E3A34FEEEB9DB", "fixSTAmountCanonicalize"),
    ("47C3002ABA31628447E8E9A8B315FAA935CE30183F9A9B86845E469CA2CDC3DF", "DisallowIncoming"),
    ("4F46DF03559967AC60F2EB272FEFE3928A7594A45FF774B87A7E540DB0F8F068", "fixAmendmentMajorityCalc"),
    ("56B241D7A43D40354D02A9DC4C8DF5C7A1F930D92A9035C4E12291B3CA3E1C2B", "Clawback"),
    ("586480873651E106F1D6339B0C4A8945BA705A777F3F4524626FF1FC07EFE41D", "MultiSignReserve"),
    ("58BE9B5968C4DA7C59BA900961828B113E5490699B21877DEF9A31E9D0FE5D5F", "fix1623"),
    ("5D08145F0A4983F23AFFFF514E83FAD355C5ABFBB6CAB76FB5BC8519FF5F33BE", "fix1515"),
    ("621A0B264970359869E3C0363A899909AAB7A887C8B73519E4ECF952D33258A8", "fixPayChanRecipientOwnerDir"),
    ("677E401A423E3708363A36BA8B3A7D019D21AC5ABD00387BDBEA6BDE4C91247E", "PermissionedDEX"),
    ("67A34F2CF55BFC0F93AACD5B281413176FEE195269FA6D95219A2DF738671172", "fix1513"),
    ("7117E2EC2DBF119CA55181D69819F1999ECEE1A0225A7FD2B9ED47940968479C", "fix1571"),
    ("726F944886BCDF7433203787E93DD9AA87FAB74DFE3AF4785BA03BEFC97ADA1F", "AMMClawback"),
    ("73761231F7F3D94EC3D8C63D91BDD0D89045C6F71B917D1925C01253515A6669", "fixNonFungibleTokensV1_2"),
    ("740352F2412A9909880C23A559FCECEDA3BE2126FED62FC7660D628A06927F11", "Flow"),
    ("755C971C29971C9F20C6F080F2ED96F87884E40AD19554A5EBECDCEC8A1F77FE", "fixEmptyDID"),
    ("75A7E01C505DD5A179DFE3E000A9B6F1EDDEB55A12F95579A23E15B15DC8BE5A", "ImmediateOfferKilled"),
    ("763C37B352BE8C7A04E810F8E462644C45AFEAD624BF3894A08E5C917CF9FF39", "fixEnforceNFTokenTrustline"),
    ("7BB62DC13EC72B775091E9C71BF8CF97E122647693B50C5E87A80DFD6FCFAC50", "fixPreviousTxnID"),
    ("7CA70A7674A26FA517412858659EBC7EDEEF7D2D608824464E6FDEFD06854E14", "fixAMMv1_3"),
    ("83FD6594FF83C1D105BD2B41D7E242D86ECB4A8220BD9AF4DA35CB0F69E39B2A", "fixFrozenLPTokenTransfer"),
    ("89308AF3B8B10B7192C4E613E1D2E4D9BA64B2EE2D5232402AE82A6A7220D953", "fixQualityUpperBound"),
    ("894646DD5284E97DECFE6674A6D6152686791C4A95F8C132CCA9BAF9E5812FB6", "Batch"),
    ("8CC0774A3BF66D1D22E76BBDA8E8A232E6B6313834301B3B23E8601196AE6455", "AMM"),
    ("8EC4304A06AF03BE953EA6EDA494864F6F3F30AA002BABA35869FBB8C6AE5D52", "fixInvalidTxFlags"),
    ("8F81B066ED20DAECA20DF57187767685EEF3980B228E0667A650BAF24426D3B4", "fixCheckThreading"),
    ("9196110C23EA879B4229E51C286180C7D02166DA712559F634372F5264D0EC59", "fixInnerObjTemplate2"),
    ("950AE2EA4654E47F04AA8739C0B214E242097E802FD372D24047A89AB1F5EC38", "MPTokensV1"),
    ("955DF3FA5891195A9DAEFA1DDC6BB244B545DDE1BAA84CBB25D5F12A8DA68A0C", "TicketBatch"),
    ("96FD2F293A519AE1DB6F8BED23E4AD9119342DA7CB6BAFD00953D16C54205D8B", "PriceOracle"),
    ("98DECF327BF79997AEC178323AD51A830E457BFC6D454DAF3E46E5EC42DC619F", "CheckCashMakesTrustLine"),
    ("A730EB18A9D4BB52502C898589558B4CCEB4BE10044500EE5581137A2E80E849", "PermissionedDomains"),
    ("AE35ABDEFBDE520372B31C957020B34A7A4A9DC3115A69803A44016477C84D6E", "fixNFTokenRemint"),
    ("AF8DF7465C338AE64B1E937D6C8DA138C0D63AD5134A68792BBBE1F63356C422", "FlowSortStrands"),
    ("B2A4DB846F0891BF2C76AB2F2ACC8F5B4EC64437135C6E56F3F859DE5FFD5856", "ExpandedSignerList"),
    ("B32752F7DCC41FB86534118FC4EEC8F56E7BD0A7DB60FD73F93F257233C08E3A", "fixEnforceNFTokenTrustlineV2"),
    ("B4E4F5D2D6FB84DF7399960A732309C9FD530EAE5941838160042833625A6076", "NegativeUNL"),
    ("B6B3EEDC0267AB50491FDC450A398AF30DBCD977CECED8BEF2499CAB5DAC19E2", "fixRmSmallIncreasedQOffers"),
    ("C1CE18F2A268E6A849C27B3DE485006771B4C01B2FCEC4F18356FE92ECD6BB74", "DynamicNFT"),
    ("C393B3AEEBF575E475F0C60D5E4241B2070CC4D0EB6C4846B1A07508FAEFC485", "fixInnerObjTemplate"),
    ("C4483A1896170C66C098DEA5B0E024309C60DC960DE5F01CD7AF986AA3D9AD37", "fixMasterKeyAsRegularKey"),
    ("C7981B764EC4439123A86CC7CCBA436E9B3FF73B3F10A0AE51882E404522FC41", "fixNFTokenPageLinks"),
    ("CA7C02118BA27599528543DFE77BA6838D1B0F43B447D4D7F53523CE6A0E9AC2", "fix1543"),
    ("D3456A862DC07E382827981CA02E21946E641877F19B8889031CC57FDCAC83E2", "fixPayChanCancelAfter"),
    ("DAF3A6EB04FA5DC51E8E4F23E9B7022B693EFA636F23F22664746C77B5786B23", "DeepFreeze"),
    ("DB432C3A09D9D5DFC7859F39AE5FF767ABC59AED0A9FB441E83B814D8946C109", "DID"),
    ("DF8B4536989BDACE3F934F29423848B9F1D76D09BE6A1FCFE7E7F06AA26ABEAD", "fixRemoveNFTokenAutoTrustLine"),
    ("EE3CF852F0506782D05E65D49E5DCC3D16D50898CD1B646BAE274863401CC3CE", "NFTokenMintOffer"),
    ("F1ED6B4A411D8B872E65B9DCB4C8B100375B0DD3D62D07192E011D6D7F339013", "fixTrustLinesToSelf"),
    ("F64E1EABBE79D55B3BB82020516CEC2C582A98A6BFE20FBE9BB6A0D233418064", "DepositAuth"),
    ("FBD513F1B893AC765B78F250E6FFA6A11B573209D1842ADC787C850696741288", "fix1578"),
]


def generate_network_config(plan, rippled_count, goxrpl_count):
    """Generate shared network configuration for all nodes.

    Creates per-node config files with validator keys, peer lists, and
    shared validator trust files so all nodes form a single private network.

    Args:
        plan: Kurtosis plan object.
        rippled_count: Number of rippled nodes.
        goxrpl_count: Number of go-xrpl nodes.

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

    # The trusted UNL includes ALL nodes (rippled + goxrpl). With issue #401's
    # bootstrap fixes (closedLedger no-regress + key-type Verify dispatch) on
    # the go-xrpl side, go-xrpl validators are first-class participants — quorum
    # is computed against the full set so go-xrpl's emitted validations
    # actually count toward fully-validated ledger advancement.
    all_pubkeys = [VALIDATOR_KEYS[i]["pubkey"] for i in range(total)]

    config_files = {}

    # Per-node rippled configs
    for i in range(rippled_count):
        peers = [name for name in all_names if name != rippled_names[i]]
        config_files["rippled-{}.cfg".format(i)] = _render_rippled_config(
            index = i,
            node_key = VALIDATOR_KEYS[i],
            peers = peers,
            rippled_count = rippled_count,
            total_validators = total,
        )

    # Per-node go-xrpl configs
    for i in range(goxrpl_count):
        key_index = rippled_count + i
        peers = [name for name in all_names if name != goxrpl_names[i]]
        config_files["goxrpl-{}.toml".format(i)] = _render_goxrpl_config(
            index = i,
            node_key = VALIDATOR_KEYS[key_index],
            peers = peers,
        )

    # Shared validators files — full UNL (rippled + goxrpl).
    config_files["validators.txt"] = _render_validators_txt(all_pubkeys)
    config_files["validators.toml"] = _render_validators_toml(all_pubkeys)

    # Genesis ledger definition for go-xrpl: enables the full advanced
    # amendment set at genesis so it matches rippled --start byte-for-byte.
    config_files["genesis.json"] = _render_goxrpl_genesis()

    return plan.render_templates(
        name = "network-config",
        config = {
            name: struct(template = content, data = {})
            for name, content in config_files.items()
        },
    )


def _render_rippled_config(index, node_key, peers, rippled_count, total_validators):
    """Render a complete rippled.cfg for a private test network node.

    Quorum is sized over the full UNL (rippled + go-xrpl). Formula:
    ceil(0.8 * total_validators), matching rippled's default 80% rule
    (RCLConsensus uses ceil, not floor+1 — for total=5 this is 4).
    """
    quorum = (total_validators * 8 + 9) // 10
    if quorum < 1:
        quorum = 1
    _ = rippled_count

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

[database_path]
/var/lib/rippled/db

[debug_logfile]
/var/lib/rippled/db/debug.log

[node_size]
tiny

[peers_max]
21

[ips_fixed]
{peers}

[peer_private]
0

[network_id]
{network_id}

[validation_quorum]
{quorum}

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
{{"command": "log_level", "severity": "info"}}
{{"command": "log_level", "partition": "LedgerConsensus", "severity": "debug"}}
{{"command": "log_level", "partition": "Ledger", "severity": "debug"}}
{{"command": "log_level", "partition": "TransactionEngine", "severity": "debug"}}
{{"command": "log_level", "partition": "Validations", "severity": "debug"}}
{{"command": "log_level", "partition": "TxQ", "severity": "debug"}}

[ledger_history]
full

[ssl_verify]
0

{amendments}""".format(
        peer_port = PEER_PORT,
        rpc_port = RPC_PORT,
        ws_port = WS_PORT,
        peers = peers_section,
        network_id = NETWORK_ID,
        quorum = quorum,
        seed = node_key["seed"],
        amendments = _render_rippled_amendments(),
    )


def _render_goxrpl_config(index, node_key, peers):
    """Render a complete go-xrpl xrpld.toml for a private test network node."""
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
# Advertise the ledgerreplay X-Protocol-Ctl feature so rippled peers
# will accept our mtREPLAY_DELTA_REQUEST and send us mtREPLAY_DELTA_RESPONSE.
ledger_replay = 1
ssl_verify = 0
genesis_amendments_disabled = false
genesis_file = "/etc/goxrpl/genesis.json"

database_path = "/tmp/goxrpl/db"
debug_logfile = "/tmp/goxrpl/db/debug.log"

node_size = "tiny"
signing_support = true
beta_rpc_api = 0

validation_seed = "{seed}"
validators_file = "/etc/goxrpl/validators.toml"

rpc_startup = [
    {{ command = "log_level", severity = "warning" }}
]

[logging]
level = "debug"
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
    """Render a validators.toml file for go-xrpl (TOML format)."""
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


def _render_rippled_amendments():
    """Render the rippled [amendments] config section.

    Lists every amendment in GENESIS_AMENDMENTS as "<64-hex-id> <Name>" so
    rippled votes them up. At --start (FRESH) rippled builds the genesis
    Amendments SLE from getDesired() = supported amendments voted up, which
    then equals exactly GENESIS_AMENDMENTS (the go-xrpl genesis set).
    """
    lines = "[amendments]\n"
    for entry in GENESIS_AMENDMENTS:
        lines += "{} {}\n".format(entry[0], entry[1])
    return lines


def _render_goxrpl_genesis():
    """Render the go-xrpl genesis.json enabling the full advanced amendment set.

    Mirrors genesis.DefaultConfig(): the master-passphrase account holds the
    full 100 billion XRP (no initial accounts, so go-xrpl funds the master
    account itself), legacy FeeSettings (BaseFee=10 drops, ReserveBase=10 XRP,
    ReserveIncrement=2 XRP — XRPFees is deliberately excluded), and
    close_time_resolution=10. totalCoins is omitted so go-xrpl falls back to
    InitialXRP and skips the balance cross-check. Only the amendment list
    differs from the built-in default. Amendment IDs are emitted in ascending
    order (GENESIS_AMENDMENTS is pre-sorted) so the serialized Amendments SLE
    is byte-identical to rippled's sorted STVector256.

    Braces are single (no Go-template actions) so the file passes through
    plan.render_templates verbatim, exactly like the rippled rpc_startup JSON.
    """
    amendments_json = ""
    n = len(GENESIS_AMENDMENTS)
    for i in range(n):
        comma = "," if i < n - 1 else ""
        amendments_json += '          "' + GENESIS_AMENDMENTS[i][0] + '"' + comma + "\n"

    head = """\
{
  "ledger": {
    "accepted": true,
    "closed": true,
    "close_time_resolution": 10,
    "accountState": [
      {
        "LedgerEntryType": "FeeSettings",
        "BaseFee": "A",
        "ReferenceFeeUnits": 10,
        "ReserveBase": 10000000,
        "ReserveIncrement": 2000000,
        "Flags": 0,
        "index": "4BC50C9B0D8515D3EAAE1E74B29A95804346C491EE1A95BF25E4AAB854A6A651"
      },
      {
        "LedgerEntryType": "Amendments",
        "Flags": 0,
        "index": "7DB0788C020F02780A673DC74757F23823FA3014C1866E72CC4CD8B226CD6EF4",
        "Amendments": [
"""
    tail = """\
        ]
      }
    ],
    "transactions": []
  }
}
"""
    return head + amendments_json + tail
