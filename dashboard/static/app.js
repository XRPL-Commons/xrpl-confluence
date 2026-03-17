(() => {
  "use strict";

  const MAX_TIMELINE = 80;
  let prevLedgerSeqs = {};
  let timelineEntries = [];

  // SSE connection
  function connectSSE() {
    const es = new EventSource("/events");
    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data);
        update(data);
      } catch {}
    };
    es.onerror = () => {
      es.close();
      setTimeout(connectSSE, 3000);
    };
  }

  // Fallback polling if SSE fails
  async function poll() {
    try {
      const res = await fetch("/api/nodes");
      const data = await res.json();
      update(data);
    } catch {}
  }

  function update(data) {
    const nodes = data.nodes || [];
    updateSyncBadge(nodes);
    updateNodeCards(nodes);
    updateTopology(nodes);
    updateTimeline(nodes);
    updateConsensus(nodes);
  }

  // Sync badge
  function updateSyncBadge(nodes) {
    const badge = document.getElementById("sync-badge");
    const label = document.getElementById("sync-label");
    const okNodes = nodes.filter((n) => n.status === "ok" && n.validated_ledger);
    if (okNodes.length === 0) {
      badge.className = "sync-badge pending";
      label.textContent = "Connecting...";
      return;
    }
    const seqs = okNodes.map((n) => n.validated_ledger.seq);
    const maxSeq = Math.max(...seqs);
    const minSeq = Math.min(...seqs);
    if (maxSeq - minSeq <= 1) {
      badge.className = "sync-badge synced";
      label.textContent = "Network Synced";
    } else {
      badge.className = "sync-badge desynced";
      label.textContent = `Desync: ${maxSeq - minSeq} ledgers apart`;
    }
  }

  // Node cards
  function updateNodeCards(nodes) {
    const grid = document.getElementById("node-grid");
    grid.innerHTML = "";
    for (const node of nodes) {
      grid.appendChild(createNodeCard(node));
    }
  }

  function createNodeCard(node) {
    const card = document.createElement("div");
    card.className = `node-card ${node.type}`;

    const statusClass =
      node.status === "ok"
        ? node.server_state === "full"
          ? "ok"
          : "warn"
        : "err";
    const stateText =
      node.status === "ok"
        ? node.server_state
        : node.status === "unreachable"
          ? "unreachable"
          : node.error || "error";

    const ledgerSeq = node.validated_ledger ? node.validated_ledger.seq : "-";
    const ledgerAge = node.validated_ledger ? `${node.validated_ledger.age}s ago` : "-";
    const peers = node.status === "ok" ? node.peers : "-";
    const uptime = node.status === "ok" ? formatUptime(node.uptime) : "-";
    const version = node.status === "ok" ? (node.build_version || "-") : "-";

    card.innerHTML = `
      <div class="node-card-header">
        <span class="node-name">${node.name}</span>
        <span class="node-type-badge ${node.type}">${node.type}</span>
      </div>
      <div class="node-status">
        <span class="status-dot ${statusClass}"></span>
        <span>${stateText}</span>
        <span style="margin-left:auto;color:var(--text-dim);font-size:11px">${version}</span>
      </div>
      <div class="node-stats">
        <div class="stat">
          <span class="stat-label">Ledger</span>
          <span class="stat-value ledger-seq">${ledgerSeq}</span>
        </div>
        <div class="stat">
          <span class="stat-label">Age</span>
          <span class="stat-value">${ledgerAge}</span>
        </div>
        <div class="stat">
          <span class="stat-label">Peers</span>
          <span class="stat-value">${peers}</span>
        </div>
        <div class="stat">
          <span class="stat-label">Uptime</span>
          <span class="stat-value">${uptime}</span>
        </div>
      </div>
    `;
    return card;
  }

  function formatUptime(seconds) {
    if (!seconds && seconds !== 0) return "-";
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return `${h}h ${m}m`;
  }

  // Topology
  function updateTopology(nodes) {
    const svg = document.getElementById("topology-svg");
    if (!svg || nodes.length === 0) return;

    const W = 700, H = 300;
    const cx = W / 2, cy = H / 2;
    const R = Math.min(W, H) / 2 - 50;
    const n = nodes.length;

    // Position nodes in a circle
    const positions = nodes.map((_, i) => {
      const angle = (2 * Math.PI * i) / n - Math.PI / 2;
      return { x: cx + R * Math.cos(angle), y: cy + R * Math.sin(angle) };
    });

    let html = "";

    // Draw links (all-to-all mesh)
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const active =
          nodes[i].status === "ok" && nodes[j].status === "ok" ? " active" : "";
        html += `<line class="topo-link${active}" x1="${positions[i].x}" y1="${positions[i].y}" x2="${positions[j].x}" y2="${positions[j].y}"/>`;
      }
    }

    // Draw nodes
    for (let i = 0; i < n; i++) {
      const node = nodes[i];
      const p = positions[i];
      const cls = node.status !== "ok" ? "unreachable" : node.type;
      const seq = node.validated_ledger ? `#${node.validated_ledger.seq}` : "";
      html += `<g class="topo-node">`;
      html += `<circle class="topo-node-circle ${cls}" cx="${p.x}" cy="${p.y}" r="24"/>`;
      html += `<text class="topo-node-label" x="${p.x}" y="${p.y}">${node.name.replace("rippled-", "R").replace("goxrpl-", "G")}</text>`;
      html += `<text class="topo-node-seq" x="${p.x}" y="${p.y + 38}">${seq}</text>`;
      html += `</g>`;
    }

    svg.innerHTML = html;
  }

  // Timeline
  function updateTimeline(nodes) {
    const container = document.getElementById("timeline-list");
    if (!container) return;

    const now = new Date();
    for (const node of nodes) {
      if (node.status !== "ok" || !node.validated_ledger) continue;
      const seq = node.validated_ledger.seq;
      const prev = prevLedgerSeqs[node.name];
      if (prev !== seq) {
        prevLedgerSeqs[node.name] = seq;
        if (prev !== undefined) {
          timelineEntries.unshift({
            time: now,
            name: node.name,
            type: node.type,
            seq: seq,
            hash: node.validated_ledger.hash || "",
          });
        }
      }
    }

    if (timelineEntries.length > MAX_TIMELINE) {
      timelineEntries = timelineEntries.slice(0, MAX_TIMELINE);
    }

    if (timelineEntries.length === 0) {
      container.innerHTML =
        '<div class="timeline-empty">Waiting for ledger closes...</div>';
      return;
    }

    container.innerHTML = timelineEntries
      .map(
        (e) => `
      <div class="timeline-entry">
        <span class="timeline-time">${formatTime(e.time)}</span>
        <span class="timeline-node ${e.type}">${e.name}</span>
        <span class="timeline-event">
          Validated ledger <span class="seq">#${e.seq}</span>
          <span class="hash">${e.hash ? e.hash.slice(0, 12) + "..." : ""}</span>
        </span>
      </div>`
      )
      .join("");
  }

  function formatTime(d) {
    return d.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }

  // Consensus metrics
  function updateConsensus(nodes) {
    const okNodes = nodes.filter((n) => n.status === "ok" && n.validated_ledger);
    const totalNodes = nodes.length;
    const onlineNodes = nodes.filter((n) => n.status === "ok").length;

    document.getElementById("metric-online").textContent = `${onlineNodes}/${totalNodes}`;
    document.getElementById("metric-validators").textContent =
      okNodes.length > 0 && okNodes[0].last_close
        ? okNodes[0].last_close.proposers || "-"
        : "-";

    const seqs = okNodes.map((n) => n.validated_ledger.seq);
    document.getElementById("metric-ledger").textContent =
      seqs.length > 0 ? Math.max(...seqs) : "-";

    const convergeTimes = okNodes
      .filter((n) => n.last_close && n.last_close.converge_time_s !== undefined)
      .map((n) => n.last_close.converge_time_s);
    document.getElementById("metric-converge").textContent =
      convergeTimes.length > 0
        ? `${(convergeTimes.reduce((a, b) => a + b, 0) / convergeTimes.length).toFixed(1)}s`
        : "-";
  }

  // Init
  document.addEventListener("DOMContentLoaded", () => {
    connectSSE();
    setInterval(poll, 5000); // fallback polling
  });
})();
