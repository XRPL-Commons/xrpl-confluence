(() => {
  "use strict";

  const MAX_TIMELINE = 80;
  // Track validated and closed seqs separately. Mixing them in a single
  // map causes seq to appear to regress (e.g. closed=6 logged, then a
  // genuine validated=4 arrives with its hash — looks like "6 → 4"
  // when really the 6 never had quorum behind it.)
  let prevValidatedSeqs = {};
  let prevClosedSeqs = {};
  let timelineEntries = [];
  let selectedNode = null;
  let logPollInterval = null;

  // Inline SVG logos
  const LOGO_CPP = `<svg viewBox="0 0 32 32" class="node-logo"><rect x="2" y="4" width="28" height="24" rx="4" fill="#659AD2"/><rect x="2" y="4" width="28" height="12" rx="4" fill="#00599C"/><rect x="2" y="12" width="28" height="4" fill="#00599C"/><text x="16" y="20" text-anchor="middle" font-family="Arial,sans-serif" font-weight="bold" font-size="12" fill="#fff">C++</text></svg>`;
  const LOGO_GO = `<img src="/gopher.svg" class="node-logo" alt="Go">`;

  function logoFor(type) {
    return type === "rippled" ? LOGO_CPP : LOGO_GO;
  }

  function topoLogoFor(type, cx, cy) {
    if (type === "rippled") {
      return `<g transform="translate(${cx - 10}, ${cy - 8})"><rect x="0" y="1" width="20" height="15" rx="3" fill="#00599C"/><text x="10" y="11" text-anchor="middle" font-family="Arial,sans-serif" font-weight="bold" font-size="8" fill="#fff">C++</text></g>`;
    }
    return `<image href="/gopher.svg" x="${cx - 14}" y="${cy - 16}" width="28" height="32"/>`;
  }

  // ── Node selection & log panel ──

  function selectNode(name) {
    selectedNode = name;
    const panel = document.getElementById("log-panel");
    panel.classList.add("open");
    document.getElementById("log-panel-title").textContent = name;
    fetchLogs(name);

    // Highlight selected card
    document.querySelectorAll(".node-card").forEach((c) => {
      c.classList.toggle("selected", c.dataset.name === name);
    });

    // Start polling logs for selected node
    if (logPollInterval) clearInterval(logPollInterval);
    logPollInterval = setInterval(() => fetchLogs(name), 2000);
  }

  function deselectNode() {
    selectedNode = null;
    const panel = document.getElementById("log-panel");
    panel.classList.remove("open");
    document.querySelectorAll(".node-card").forEach((c) => c.classList.remove("selected"));
    if (logPollInterval) {
      clearInterval(logPollInterval);
      logPollInterval = null;
    }
  }

  async function fetchLogs(name) {
    try {
      const res = await fetch(`/api/logs/${encodeURIComponent(name)}`);
      const data = await res.json();
      renderLogPanel(data);
    } catch {
      document.getElementById("log-panel-logs").innerHTML =
        '<div class="log-empty">Failed to fetch logs.</div>';
    }
  }

  function renderLogPanel(data) {
    const stateEl = document.getElementById("log-panel-state");
    const logsEl = document.getElementById("log-panel-logs");

    // Render current state as formatted JSON
    if (data.state) {
      const s = data.state;
      const seq = s.validated_ledger
        ? `validated #${s.validated_ledger.seq}`
        : s.closed_ledger
          ? `closed #${s.closed_ledger.seq}`
          : s.ledger_current_index
            ? `current #${s.ledger_current_index}`
            : "—";
      const stateClass = s.status === "ok" ? (["proposing", "full", "validating"].includes(s.server_state) ? "ok" : "warn") : "err";

      stateEl.innerHTML = `
        <div class="log-state-row">
          <span class="status-dot ${stateClass}"></span>
          <strong>${s.server_state || s.status}</strong>
          <span class="log-state-dim">${seq}</span>
          <span class="log-state-dim">peers: ${s.peers ?? "—"}</span>
          <span class="log-state-dim">${s.build_version || ""}</span>
        </div>
        <div class="log-state-row log-state-dim">
          complete: ${s.complete_ledgers || "—"}
          ${s.last_close ? `| proposers: ${s.last_close.proposers} | converge: ${s.last_close.converge_time_s}s` : ""}
        </div>
      `;
    } else {
      stateEl.innerHTML = '<span class="log-state-dim">No state available</span>';
    }

    // Render log entries (newest first)
    if (!data.logs || data.logs.length === 0) {
      logsEl.innerHTML = '<div class="log-empty">No log entries yet.</div>';
      return;
    }

    const entries = data.logs.slice().reverse();
    logsEl.innerHTML = entries
      .map((e) => {
        const time = e.ts.split("T")[1]?.split(".")[0] || e.ts;
        const levelClass =
          e.level === "unreachable" || e.level === "error"
            ? "log-err"
            : e.level === "proposing" || e.level === "validating"
              ? "log-ok"
              : "log-info";
        return `<div class="log-line ${levelClass}"><span class="log-ts">${time}</span><span class="log-level">${e.level}</span><span class="log-msg">${e.message}</span></div>`;
      })
      .join("");

    // Auto-scroll to top (newest)
    logsEl.scrollTop = 0;
  }

  // ── SSE & polling ──

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

  // ── Sync badge ──

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

  // ── Node cards ──

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
    if (selectedNode === node.name) card.classList.add("selected");
    card.dataset.name = node.name;
    card.style.cursor = "pointer";
    card.addEventListener("click", () => selectNode(node.name));

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

  // ── Topology ──

  function updateTopology(nodes) {
    const svg = document.getElementById("topology-svg");
    if (!svg || nodes.length === 0) return;

    const W = 700, H = 340;
    const cx = W / 2, cy = H / 2;
    const R = Math.min(W, H) / 2 - 50;
    const n = nodes.length;

    const positions = nodes.map((_, i) => {
      const angle = (2 * Math.PI * i) / n - Math.PI / 2;
      return { x: cx + R * Math.cos(angle), y: cy + R * Math.sin(angle) };
    });

    let html = "";

    // Links
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const active =
          nodes[i].status === "ok" && nodes[j].status === "ok" ? " active" : "";
        html += `<line class="topo-link${active}" x1="${positions[i].x}" y1="${positions[i].y}" x2="${positions[j].x}" y2="${positions[j].y}"/>`;
      }
    }

    // Nodes (clickable)
    for (let i = 0; i < n; i++) {
      const node = nodes[i];
      const p = positions[i];
      const cls = node.status !== "ok" ? "unreachable" : node.type;
      const isSelected = selectedNode === node.name;
      const seqVal = node.validated_ledger ? node.validated_ledger.seq : (node.ledger_current_index || (node.closed_ledger ? node.closed_ledger.seq : null));
      const seq = seqVal ? `#${seqVal}` : "";
      const shortName = node.name.replace("rippled-", "R").replace("goxrpl-", "G");
      html += `<g class="topo-node clickable" data-name="${node.name}">`;
      html += `<circle class="topo-node-circle ${cls}${isSelected ? " selected" : ""}" cx="${p.x}" cy="${p.y}" r="24"/>`;
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

    // Attach click handlers to topology nodes
    svg.querySelectorAll(".topo-node.clickable").forEach((g) => {
      g.addEventListener("click", () => {
        const name = g.dataset.name;
        if (name) selectNode(name);
      });
    });
  }

  // ── Timeline ──

  function updateTimeline(nodes) {
    const container = document.getElementById("timeline-list");
    if (!container) return;

    const now = new Date();
    for (const node of nodes) {
      if (node.status !== "ok") continue;

      // Validated seq advances become "Validated ledger #N" entries.
      if (node.validated_ledger && node.validated_ledger.seq) {
        const vseq = node.validated_ledger.seq;
        const prev = prevValidatedSeqs[node.name];
        if (prev !== vseq) {
          prevValidatedSeqs[node.name] = vseq;
          if (prev !== undefined) {
            timelineEntries.unshift({
              time: now,
              name: node.name,
              type: node.type,
              seq: vseq,
              hash: node.validated_ledger.hash || "",
              kind: "validated",
            });
          }
        }
      }

      // Closed seq advances — only log while we don't have a validated
      // ledger yet, so the pre-quorum ramp-up is visible without
      // conflating with the real "validated" events. Once the node has
      // a validated_ledger, subsequent closed advances are redundant.
      if (!node.validated_ledger && node.closed_ledger && node.closed_ledger.seq) {
        const cseq = node.closed_ledger.seq;
        const prev = prevClosedSeqs[node.name];
        if (prev !== cseq) {
          prevClosedSeqs[node.name] = cseq;
          if (prev !== undefined) {
            timelineEntries.unshift({
              time: now,
              name: node.name,
              type: node.type,
              seq: cseq,
              hash: "",
              kind: "closed",
            });
          }
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
      .map((e) => {
        const verb = e.kind === "closed" ? "Closed" : "Validated";
        return `
      <div class="timeline-entry">
        <span class="timeline-time">${formatTime(e.time)}</span>
        <span class="timeline-node ${e.type}">${e.name}</span>
        <span class="timeline-event">
          ${verb} ledger <span class="seq">#${e.seq}</span>
          <span class="hash">${e.hash ? e.hash.slice(0, 12) + "..." : ""}</span>
        </span>
      </div>`;
      })
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

  // ── Consensus metrics ──

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

  // ── Fuzzer panel ──

  async function pollFuzz() {
    try {
      const r = await fetch("/api/fuzz");
      if (r.status === 204) return;
      if (!r.ok) return;
      const data = await r.json();
      document.getElementById("fuzz-submitted").textContent = data.txs_submitted_total ?? "—";
      document.getElementById("fuzz-applied").textContent = data.txs_applied_total ?? "—";
      document.getElementById("fuzz-divergences").textContent = data.divergences_total ?? "—";
      document.getElementById("fuzz-crashes").textContent = data.crashes_total ?? "—";
      document.getElementById("fuzz-seed").textContent = data.current_seed ?? "—";
      const tbody = document.querySelector("#fuzz-by-layer tbody");
      tbody.innerHTML = "";
      for (const [layer, count] of Object.entries(data.divergences_total_by_layer ?? {})) {
        const tr = document.createElement("tr");
        tr.innerHTML = `<td>${layer}</td><td>${count}</td>`;
        tbody.appendChild(tr);
      }
    } catch (_) {
      // panel stays at "—" until the sidecar comes up.
    }
  }

  // ── Init ──

  document.addEventListener("DOMContentLoaded", () => {
    connectSSE();
    setInterval(poll, 5000);

    // Close button for log panel
    document.getElementById("log-panel-close").addEventListener("click", deselectNode);

    setInterval(pollFuzz, 5000);
    pollFuzz();
  });
})();
