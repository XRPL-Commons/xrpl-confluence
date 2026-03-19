(() => {
  "use strict";

  const MAX_TIMELINE = 80;
  let prevLedgerSeqs = {};
  let timelineEntries = [];

  // Inline SVG logos
  const LOGO_CPP = `<svg viewBox="0 0 32 32" class="node-logo"><rect x="2" y="4" width="28" height="24" rx="4" fill="#659AD2"/><rect x="2" y="4" width="28" height="12" rx="4" fill="#00599C"/><rect x="2" y="12" width="28" height="4" fill="#00599C"/><text x="16" y="20" text-anchor="middle" font-family="Arial,sans-serif" font-weight="bold" font-size="12" fill="#fff">C++</text></svg>`;
  const LOGO_GO = `<svg viewBox="0 0 32 32" class="node-logo"><circle cx="16" cy="16" r="14" fill="#00ADD8"/><circle cx="11" cy="13" r="3" fill="#fff"/><circle cx="21" cy="13" r="3" fill="#fff"/><circle cx="11" cy="13" r="1.5" fill="#000"/><circle cx="21" cy="13" r="1.5" fill="#000"/><path d="M10 21 Q16 25 22 21" stroke="#fff" stroke-width="1.5" fill="none" stroke-linecap="round"/><rect x="12" y="3" rx="1" width="3" height="5" fill="#00ADD8" transform="rotate(-15 13.5 5.5)"/><rect x="17" y="3" rx="1" width="3" height="5" fill="#00ADD8" transform="rotate(15 18.5 5.5)"/></svg>`;

  function logoFor(type) {
    return type === "rippled" ? LOGO_CPP : LOGO_GO;
  }

  // Small logo for topology nodes (returns SVG content to embed inside the main SVG)
  function topoLogoFor(type, cx, cy) {
    if (type === "rippled") {
      return `<g transform="translate(${cx - 10}, ${cy - 8})"><rect x="0" y="1" width="20" height="15" rx="3" fill="#00599C"/><text x="10" y="11" text-anchor="middle" font-family="Arial,sans-serif" font-weight="bold" font-size="8" fill="#fff">C++</text></g>`;
    }
    return `<g transform="translate(${cx - 9}, ${cy - 9})"><circle cx="9" cy="9" r="9" fill="#00ADD8"/><circle cx="6.5" cy="7.5" r="1.8" fill="#fff"/><circle cx="11.5" cy="7.5" r="1.8" fill="#fff"/><circle cx="6.5" cy="7.5" r="0.9" fill="#000"/><circle cx="11.5" cy="7.5" r="0.9" fill="#000"/><path d="M5.5 12.5 Q9 15 12.5 12.5" stroke="#fff" stroke-width="1" fill="none" stroke-linecap="round"/></g>`;
  }

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
    const okNodes = nodes.filter((n) => n.status === "ok" && (n.validated_ledger || n.closed_ledger || n.ledger_current_index));
    if (okNodes.length === 0) {
      badge.className = "sync-badge pending";
      label.textContent = "Connecting...";
      return;
    }
    const seqs = okNodes.map((n) => {
      if (n.validated_ledger) return n.validated_ledger.seq;
      if (n.closed_ledger) return n.closed_ledger.seq;
      return n.ledger_current_index;
    });
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

    const healthyStates = ["full", "proposing", "validating"];
    const statusClass =
      node.status === "ok"
        ? healthyStates.includes(node.server_state)
          ? "ok"
          : "warn"
        : "err";
    const stateText =
      node.status === "ok"
        ? node.server_state
        : node.status === "unreachable"
          ? "unreachable"
          : node.error || "error";

    const validatedSeq = node.validated_ledger ? node.validated_ledger.seq : null;
    const currentSeq = node.ledger_current_index || (node.closed_ledger ? node.closed_ledger.seq : null);
    const ledgerSeq = validatedSeq || currentSeq || "-";
    const ledgerAge = node.validated_ledger ? `${node.validated_ledger.age}s ago` : "-";
    const peers = node.status === "ok" ? node.peers : "-";
    const uptime = node.status === "ok" ? formatUptime(node.uptime) : "-";
    const version = node.status === "ok" ? (node.build_version || "-") : "-";

    card.innerHTML = `
      <div class="node-card-header">
        <div class="node-name-group">
          ${logoFor(node.type)}
          <span class="node-name">${node.name}</span>
        </div>
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

    const W = 700, H = 340;
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
      const seqVal = node.validated_ledger ? node.validated_ledger.seq : (node.ledger_current_index || (node.closed_ledger ? node.closed_ledger.seq : null));
      const seq = seqVal ? `#${seqVal}` : "";
      const shortName = node.name.replace("rippled-", "R").replace("goxrpl-", "G");
      html += `<g class="topo-node">`;
      html += `<circle class="topo-node-circle ${cls}" cx="${p.x}" cy="${p.y}" r="24"/>`;
      if (node.status === "ok") {
        html += topoLogoFor(node.type, p.x, p.y);
      } else {
        html += `<text class="topo-node-label" x="${p.x}" y="${p.y}">${shortName}</text>`;
      }
      html += `<text class="topo-node-name" x="${p.x}" y="${p.y + 34}">${node.name}</text>`;
      html += `<text class="topo-node-seq" x="${p.x}" y="${p.y + 46}">${seq}</text>`;
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
      if (node.status !== "ok") continue;
      const seq = node.validated_ledger ? node.validated_ledger.seq
        : (node.ledger_current_index || (node.closed_ledger ? node.closed_ledger.seq : null));
      if (!seq) continue;
      const prev = prevLedgerSeqs[node.name];
      if (prev !== seq) {
        prevLedgerSeqs[node.name] = seq;
        if (prev !== undefined) {
          const hash = node.validated_ledger ? (node.validated_ledger.hash || "") : "";
          timelineEntries.unshift({
            time: now,
            name: node.name,
            type: node.type,
            seq: seq,
            hash: hash,
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
    const okNodes = nodes.filter((n) => n.status === "ok");
    const nodesWithLedger = okNodes.filter((n) => n.validated_ledger || n.closed_ledger || n.ledger_current_index);
    const totalNodes = nodes.length;
    const onlineNodes = okNodes.length;

    document.getElementById("metric-online").textContent = `${onlineNodes}/${totalNodes}`;
    document.getElementById("metric-validators").textContent =
      okNodes.length > 0 && okNodes[0].last_close
        ? okNodes[0].last_close.proposers || "-"
        : "-";

    const seqs = nodesWithLedger.map((n) => {
      if (n.validated_ledger) return n.validated_ledger.seq;
      if (n.closed_ledger) return n.closed_ledger.seq;
      return n.ledger_current_index;
    });
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
