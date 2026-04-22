const http = require("http");
const fs = require("fs");
const path = require("path");

const CONFIG_PATH = process.env.CONFIG_PATH || "/app/config.json";
const PORT = parseInt(process.env.PORT || "8080", 10);
const STATIC_DIR = path.join(__dirname, "static");

// Requires Node 22+ for global WebSocket (set by the Kurtosis image).
if (typeof WebSocket === "undefined") {
  console.error("FATAL: global WebSocket not available — needs node >= 22");
  process.exit(1);
}

const MIME = {
  ".html": "text/html",
  ".css": "text/css",
  ".js": "application/javascript",
  ".json": "application/json",
  ".svg": "image/svg+xml",
};

let config = { nodes: [], poll_interval_ms: 5000 };
let nodeStates = {};
// Rolling log of raw server_info responses per node (last 100 entries).
let nodeLogs = {};
const MAX_LOG_ENTRIES = 100;
let sseClients = [];
// Active XRPL subscribe-stream WS per node, keyed by node name.
// Replaced on reconnect; a dead/closed socket is never left in the map.
const wsByNode = new Map();

function loadConfig() {
  try {
    config = JSON.parse(fs.readFileSync(CONFIG_PATH, "utf8"));
    console.log(
      `Loaded config: ${config.nodes.length} nodes, poll every ${config.poll_interval_ms}ms`
    );
    for (const n of config.nodes) {
      nodeLogs[n.name] = [];
    }
  } catch (e) {
    console.error("Failed to load config:", e.message);
  }
}

function rpcCall(rpcUrl, method) {
  return new Promise((resolve, reject) => {
    const url = new URL(rpcUrl);
    const body = JSON.stringify({ method, params: [{}] });
    const req = http.request(
      {
        hostname: url.hostname,
        port: url.port,
        path: "/",
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Content-Length": Buffer.byteLength(body),
        },
        timeout: 3000,
      },
      (res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch {
            reject(new Error("Invalid JSON"));
          }
        });
      }
    );
    req.on("error", reject);
    req.on("timeout", () => {
      req.destroy();
      reject(new Error("Timeout"));
    });
    req.end(body);
  });
}

// openLedgerStream opens a persistent XRPL subscribe stream to the node
// and pushes ledgerClosed events into nodeStates as they arrive, instead
// of waiting for the next HTTP poll cycle. This eliminates the polling-
// phase artifact where one node appears to be ahead of another simply
// because its poll landed a few hundred ms earlier in the close cycle.
//
// On disconnect it reconnects with a short backoff — enough to ride out
// container restarts without hammering an unhealthy node.
function openLedgerStream(node) {
  if (!node.ws) return;
  let ws;
  try {
    ws = new WebSocket(node.ws);
  } catch (e) {
    console.error(`[ws ${node.name}] ctor failed: ${e.message}`);
    scheduleReconnect(node);
    return;
  }
  wsByNode.set(node.name, ws);

  ws.addEventListener("open", () => {
    ws.send(JSON.stringify({ command: "subscribe", streams: ["ledger"] }));
    pushLog(node.name, new Date().toISOString(), "ws", "subscribed to ledger stream");
  });

  ws.addEventListener("message", (ev) => {
    let msg;
    try {
      msg = JSON.parse(ev.data);
    } catch {
      return;
    }
    // ledgerClosed events arrive for every new validated ledger. We
    // update validated_ledger and fan out to SSE clients immediately.
    if (msg.type !== "ledgerClosed") return;

    const seq = msg.ledger_index;
    const hash = msg.ledger_hash;
    if (seq == null || !hash) return;

    const ts = new Date().toISOString();
    const prev = nodeStates[node.name] || { name: node.name, type: node.type };
    nodeStates[node.name] = {
      ...prev,
      name: node.name,
      type: node.type,
      validated_ledger: { seq, hash },
    };
    pushLog(node.name, ts, "ledger", `ws-validated=#${seq} hash=${hash.slice(0, 16)}…`);
    broadcastSnapshot();
  });

  ws.addEventListener("close", () => {
    wsByNode.delete(node.name);
    scheduleReconnect(node);
  });

  ws.addEventListener("error", () => {
    // Close handler will schedule the reconnect — no double-schedule.
    try { ws.close(); } catch {}
  });
}

function scheduleReconnect(node) {
  setTimeout(() => openLedgerStream(node), 2000);
}

function broadcastSnapshot() {
  const snapshot = JSON.stringify({
    timestamp: Date.now(),
    nodes: config.nodes.map((n) => nodeStates[n.name] || { name: n.name, status: "pending" }),
  });
  for (const client of sseClients) {
    client.write(`data: ${snapshot}\n\n`);
  }
}

async function pollNode(node) {
  const ts = new Date().toISOString();
  try {
    const resp = await rpcCall(node.rpc, "server_info");
    const info = resp.result && resp.result.info;
    if (!info) {
      nodeStates[node.name] = {
        name: node.name,
        type: node.type,
        status: "error",
        error: "No info in response",
      };
      pushLog(node.name, ts, "error", "No info in response");
      return;
    }
    // Merge HTTP fields without clobbering validated_ledger that the WS
    // stream may have pushed ahead of our poll. WS is the source of
    // truth for the flip event; HTTP fills in everything else (peers,
    // uptime, last_close, etc.) and bootstraps validated_ledger before
    // the first ledgerClosed arrives.
    const prev = nodeStates[node.name] || {};
    const wsValidated = prev.validated_ledger;
    nodeStates[node.name] = {
      name: node.name,
      type: node.type,
      status: "ok",
      server_state: info.server_state,
      build_version: info.build_version,
      uptime: info.uptime,
      peers: info.peers,
      complete_ledgers: info.complete_ledgers,
      validated_ledger: wsValidated || info.validated_ledger || null,
      closed_ledger: info.closed_ledger || null,
      ledger_current_index: info.ledger_current_index || null,
      last_close: info.last_close || null,
      network_id: info.network_id,
      pubkey_node: info.pubkey_node,
    };
    const vl = nodeStates[node.name].validated_ledger;
    const seq = vl
      ? `validated=#${vl.seq}`
      : info.closed_ledger
        ? `closed=#${info.closed_ledger.seq}`
        : info.ledger_current_index
          ? `current=#${info.ledger_current_index}`
          : "no-ledger";
    const proposers = info.last_close ? `proposers=${info.last_close.proposers}` : "";
    const converge = info.last_close ? `converge=${info.last_close.converge_time_s}s` : "";
    pushLog(node.name, ts, info.server_state, `${seq} peers=${info.peers} ${proposers} ${converge}`.trim());
  } catch (e) {
    nodeStates[node.name] = {
      name: node.name,
      type: node.type,
      status: "unreachable",
      error: e.message,
    };
    pushLog(node.name, ts, "unreachable", e.message);
  }
}

function pushLog(name, ts, level, message) {
  if (!nodeLogs[name]) nodeLogs[name] = [];
  nodeLogs[name].push({ ts, level, message });
  if (nodeLogs[name].length > MAX_LOG_ENTRIES) {
    nodeLogs[name] = nodeLogs[name].slice(-MAX_LOG_ENTRIES);
  }
}

async function pollAll() {
  await Promise.allSettled(config.nodes.map(pollNode));
  broadcastSnapshot();
}

function serveStatic(req, res) {
  let filePath = req.url === "/" ? "/index.html" : req.url;
  filePath = path.join(STATIC_DIR, filePath);
  const ext = path.extname(filePath);
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end("Not found");
      return;
    }
    res.writeHead(200, { "Content-Type": MIME[ext] || "application/octet-stream" });
    res.end(data);
  });
}

const server = http.createServer((req, res) => {
  if (req.url === "/api/nodes") {
    res.writeHead(200, {
      "Content-Type": "application/json",
      "Access-Control-Allow-Origin": "*",
    });
    res.end(
      JSON.stringify({
        timestamp: Date.now(),
        nodes: config.nodes.map(
          (n) => nodeStates[n.name] || { name: n.name, status: "pending" }
        ),
      })
    );
    return;
  }

  // Logs endpoint: GET /api/logs/<node-name>
  const logMatch = req.url.match(/^\/api\/logs\/(.+)$/);
  if (logMatch) {
    const name = decodeURIComponent(logMatch[1]);
    res.writeHead(200, {
      "Content-Type": "application/json",
      "Access-Control-Allow-Origin": "*",
    });
    res.end(JSON.stringify({
      name,
      state: nodeStates[name] || null,
      logs: nodeLogs[name] || [],
    }));
    return;
  }

  if (req.url === "/events") {
    res.writeHead(200, {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "Access-Control-Allow-Origin": "*",
    });
    res.write(":\n\n");
    sseClients.push(res);
    req.on("close", () => {
      sseClients = sseClients.filter((c) => c !== res);
    });
    return;
  }

  serveStatic(req, res);
});

loadConfig();
server.listen(PORT, "0.0.0.0", () => {
  console.log(`Dashboard running on http://0.0.0.0:${PORT}`);
  // Open a persistent XRPL subscribe stream per node. These push ledger
  // close events into nodeStates immediately; HTTP polling stays as a
  // coarser fallback for peers/state/uptime.
  for (const node of config.nodes) {
    openLedgerStream(node);
  }
  setInterval(pollAll, config.poll_interval_ms);
  pollAll();
});
