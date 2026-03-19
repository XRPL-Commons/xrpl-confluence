const http = require("http");
const fs = require("fs");
const path = require("path");

const CONFIG_PATH = process.env.CONFIG_PATH || "/app/config.json";
const PORT = parseInt(process.env.PORT || "8080", 10);
const STATIC_DIR = path.join(__dirname, "static");

const MIME = {
  ".html": "text/html",
  ".css": "text/css",
  ".js": "application/javascript",
  ".json": "application/json",
  ".svg": "image/svg+xml",
};

let config = { nodes: [], poll_interval_ms: 2000 };
let nodeStates = {};
// Rolling log of raw server_info responses per node (last 100 entries).
let nodeLogs = {};
const MAX_LOG_ENTRIES = 100;
let sseClients = [];

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
    nodeStates[node.name] = {
      name: node.name,
      type: node.type,
      status: "ok",
      server_state: info.server_state,
      build_version: info.build_version,
      uptime: info.uptime,
      peers: info.peers,
      complete_ledgers: info.complete_ledgers,
      validated_ledger: info.validated_ledger || null,
      closed_ledger: info.closed_ledger || null,
      ledger_current_index: info.ledger_current_index || null,
      last_close: info.last_close || null,
      network_id: info.network_id,
      pubkey_node: info.pubkey_node,
    };
    // Build a concise log line from the state.
    const seq = info.validated_ledger
      ? `validated=#${info.validated_ledger.seq}`
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
  const snapshot = JSON.stringify({
    timestamp: Date.now(),
    nodes: config.nodes.map((n) => nodeStates[n.name] || { name: n.name, status: "pending" }),
  });
  for (const client of sseClients) {
    client.write(`data: ${snapshot}\n\n`);
  }
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
  setInterval(pollAll, config.poll_interval_ms);
  pollAll();
});
