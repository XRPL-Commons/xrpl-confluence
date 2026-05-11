(() => {
  "use strict";

  const TABS = ["overview", "nodes", "topology", "timeline", "fuzzer"];
  const MOCK = new URLSearchParams(location.search).get("mock") === "1";

  // ── Tab routing ─────────────────────────────────────────────
  function currentTab() {
    const h = location.hash.replace("#", "");
    return TABS.includes(h) ? h : "overview";
  }

  function setTab(name) {
    if (!TABS.includes(name)) name = "overview";
    document.querySelector("main.poster").dataset.tab = name;
    for (const el of document.querySelectorAll(".tab")) {
      el.classList.toggle("active", el.dataset.tab === name);
    }
    for (const el of document.querySelectorAll(".panel")) {
      el.classList.toggle("active", el.dataset.panel === name);
    }
    if (location.hash.replace("#", "") !== name) location.hash = name;
    updateFooterContext();
  }

  // ── Data layer ──────────────────────────────────────────────
  let latest = { nodes: [] };
  let latestFuzz = null;
  let lastUpdate = 0;
  const renderers = []; // pushed by each tab module

  function notify() {
    lastUpdate = Date.now();
    for (const fn of renderers) {
      try { fn(latest, latestFuzz); } catch (e) { console.error(e); }
    }
    updateFooterContext();
  }

  async function fetchJSON(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`${url} → ${res.status}`);
    return res.json();
  }

  async function pollOnce() {
    if (MOCK) {
      latest = await fetchJSON("/fixtures/mock.json");
      latestFuzz = await fetchJSON("/fixtures/mock-fuzz.json");
      setSync("connected");
      notify();
      return;
    }
    try {
      latest = await fetchJSON("/api/nodes");
      setSync("connected");
    } catch {
      setSync("reconnecting");
    }
    try { latestFuzz = await fetchJSON("/api/fuzz"); } catch { latestFuzz = null; }
    notify();
  }

  function connectSSE() {
    if (MOCK) return;
    const es = new EventSource("/events");
    es.onmessage = (e) => {
      try {
        latest = JSON.parse(e.data);
        setSync("connected");
        notify();
      } catch {}
    };
    es.onerror = () => {
      setSync("offline");
      es.close();
      setTimeout(connectSSE, 3000);
    };
  }

  function setSync(state) {
    const badge = document.getElementById("sync-badge");
    const label = document.getElementById("sync-label");
    badge.classList.remove("connected", "reconnecting", "offline");
    badge.classList.add(state);
    label.textContent = state.toUpperCase();
  }

  // ── Overview ────────────────────────────────────────────────
  function renderOverview(data, fuzz) {
    const nodes = data.nodes || [];
    const ok = nodes.filter((n) => n.status === "ok");
    const seqs = ok
      .map((n) => n.validated_ledger?.seq ?? n.closed_ledger?.seq ?? n.ledger_current_index)
      .filter(Boolean);
    const maxSeq = seqs.length ? Math.max(...seqs) : null;
    const minSeq = seqs.length ? Math.min(...seqs) : null;
    const proposers = ok[0]?.last_close?.proposers ?? "—";
    const convergeArr = ok.map((n) => n.last_close?.converge_time_s).filter((v) => v != null);
    const converge = convergeArr.length
      ? `${(convergeArr.reduce((a, b) => a + b, 0) / convergeArr.length).toFixed(1)}s`
      : "—";

    const kpis = [
      { lbl: "Nodes online", num: `${ok.length} / ${nodes.length}` },
      { lbl: "Latest ledger", num: maxSeq ? maxSeq.toLocaleString("en-US") : "—" },
      { lbl: "Proposers", num: proposers },
      { lbl: "Converge time", num: converge },
    ];
    document.getElementById("overview-kpis").innerHTML = kpis
      .map((k) => `<div class="kpi"><div class="kpi-lbl">${k.lbl}</div><div class="kpi-num">${k.num}</div></div>`)
      .join("");

    const spread = maxSeq != null && minSeq != null ? maxSeq - minSeq : 0;
    const divergences = fuzz?.divergences_total ?? 0;
    const crashes = fuzz?.crashes_total ?? 0;
    const rows = [
      { item: "Ledger spread", status: spread <= 1 ? "synced" : `${spread} ledgers apart`, count: spread },
      { item: "Unreachable nodes", status: nodes.length - ok.length === 0 ? "none" : "needs attention", count: nodes.length - ok.length },
      { item: "Fuzzer divergences", status: divergences === 0 ? "clean" : "investigating", count: divergences },
      { item: "Fuzzer crashes", status: crashes === 0 ? "clean" : "open", count: crashes },
    ];
    document.querySelector("#overview-summary tbody").innerHTML = rows
      .map((r) => `<tr><td>${r.item}</td><td>${r.status}</td><td>${r.count}</td></tr>`)
      .join("");
  }
  renderers.push(renderOverview);

  // ── Nodes ───────────────────────────────────────────────────
  const HEALTHY = new Set(["full", "proposing", "validating"]);
  function healthClass(n) {
    if (n.status !== "ok") return "health-err";
    return HEALTHY.has(n.server_state) ? "health-ok" : "health-warn";
  }

  function renderNodes(data) {
    const grid = document.getElementById("node-grid");
    grid.innerHTML = (data.nodes || []).map((n) => {
      const seq = n.validated_ledger?.seq ?? n.closed_ledger?.seq ?? n.ledger_current_index ?? "—";
      const peers = n.status === "ok" ? n.peers ?? "—" : "—";
      const lag = n.validated_ledger?.age != null ? `${n.validated_ledger.age}s` : "—";
      const id = n.status === "ok" ? (n.build_version || "—") : (n.error || "offline");
      return `
        <article class="node-card ${healthClass(n)}" data-name="${n.name}">
          <div class="node-name">${n.name}</div>
          <div class="node-id">${id}</div>
          <div class="node-mini">
            <div><div class="l">Ledger</div><div class="v">${typeof seq === "number" ? seq.toLocaleString("en-US") : seq}</div></div>
            <div><div class="l">Peers</div><div class="v">${peers}</div></div>
            <div><div class="l">Lag</div><div class="v">${lag}</div></div>
          </div>
        </article>`;
    }).join("");

    for (const card of grid.querySelectorAll(".node-card")) {
      card.addEventListener("click", () => openDrawer(card.dataset.name));
    }
  }
  renderers.push(renderNodes);

  // ── Topology ────────────────────────────────────────────────
  function renderTopology(data) {
    const svg = document.getElementById("topology-svg");
    const nodes = data.nodes || [];
    if (!nodes.length) { svg.innerHTML = ""; return; }
    const W = 700, H = 520, cx = W / 2, cy = H / 2 - 20, R = Math.min(W, H) / 2 - 80;
    const pos = nodes.map((_, i) => {
      const a = (2 * Math.PI * i) / nodes.length - Math.PI / 2;
      return { x: cx + R * Math.cos(a), y: cy + R * Math.sin(a) };
    });

    let html = "";
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const cls = nodes[i].status === "ok" && nodes[j].status === "ok" ? "active" : "";
        html += `<line class="topo-link ${cls}" x1="${pos[i].x}" y1="${pos[i].y}" x2="${pos[j].x}" y2="${pos[j].y}"/>`;
      }
    }
    for (let i = 0; i < nodes.length; i++) {
      const n = nodes[i], p = pos[i];
      const cls = n.status !== "ok" ? "unreachable" : (HEALTHY.has(n.server_state) ? "" : "warn");
      html += `<g class="topo-node" data-name="${n.name}">
        <circle class="topo-node-circle ${cls}" cx="${p.x}" cy="${p.y}" r="14"/>
        <text class="topo-node-label" x="${p.x}" y="${p.y + 22}">${n.name}</text>
      </g>`;
    }
    svg.innerHTML = html;

    for (const g of svg.querySelectorAll(".topo-node")) {
      g.addEventListener("click", () => openDrawer(g.dataset.name));
    }
  }
  renderers.push(renderTopology);

  // ── Timeline ────────────────────────────────────────────────
  const MAX_TIMELINE = 80;
  const prevValidatedSeqs = {};
  const prevClosedSeqs = {};
  const closeRows = new Map(); // seq → { time, hash, byNode: Map<name, "match"|"diverged"> }
  let pendingNew = 0;
  let timelinePinned = false;

  function pushTimelineEvents(nodes) {
    const now = new Date();
    for (const n of nodes) {
      if (n.status !== "ok") continue;
      const v = n.validated_ledger?.seq;
      if (v != null && prevValidatedSeqs[n.name] !== v) {
        prevValidatedSeqs[n.name] = v;
        const row = closeRows.get(v) || { time: now, hash: n.validated_ledger.hash || "", byNode: new Map() };
        const hashSeen = row.hash || n.validated_ledger.hash || "";
        const diverged = hashSeen && n.validated_ledger.hash && hashSeen !== n.validated_ledger.hash;
        row.byNode.set(n.name, diverged ? "diverged" : "match");
        row.hash = hashSeen;
        if (!closeRows.has(v)) { closeRows.set(v, row); pendingNew += 1; }
      }
      const c = n.closed_ledger?.seq;
      if (!v && c != null && prevClosedSeqs[n.name] !== c) {
        prevClosedSeqs[n.name] = c;
        const row = closeRows.get(c) || { time: now, hash: "", byNode: new Map() };
        row.byNode.set(n.name, "match");
        if (!closeRows.has(c)) { closeRows.set(c, row); pendingNew += 1; }
      }
    }
    if (closeRows.size > MAX_TIMELINE) {
      const seqs = [...closeRows.keys()].sort((a, b) => a - b);
      while (seqs.length && closeRows.size > MAX_TIMELINE) {
        closeRows.delete(seqs.shift());
      }
    }
  }

  function renderTimeline(data) {
    pushTimelineEvents(data.nodes || []);
    const list = document.getElementById("timeline-list");
    const chip = document.getElementById("timeline-new-chip");
    const seqs = [...closeRows.keys()].sort((a, b) => a - b);

    if (!seqs.length) {
      list.innerHTML = `<div class="timeline-empty">Waiting for ledger closes…</div>`;
      chip.hidden = true;
      return;
    }
    const nodeNames = (data.nodes || []).map((n) => n.name);
    const html = seqs.map((seq) => {
      const row = closeRows.get(seq);
      const time = row.time.toLocaleTimeString("en-US", { hour12: false });
      const hash = row.hash ? row.hash.slice(0, 12) + "…" : "";
      const dots = nodeNames.map((name) => {
        const status = row.byNode.get(name);
        if (!status) return `<span style="background:var(--rule)"></span>`;
        return `<span class="${status === "diverged" ? "diverged" : ""}"></span>`;
      }).join("");
      return `<div class="timeline-row" data-seq="${seq}">
        <div class="timeline-seq">${seq.toLocaleString("en-US")}</div>
        <div class="timeline-rule"></div>
        <div class="timeline-meta">
          <span>${time}</span>
          <span class="timeline-hash">${hash}</span>
          <span class="timeline-dots">${dots}</span>
        </div>
      </div>`;
    }).join("");
    list.innerHTML = html;

    if (timelinePinned && pendingNew > 0) {
      document.getElementById("timeline-new-count").textContent = pendingNew;
      chip.hidden = false;
    } else {
      list.scrollTop = list.scrollHeight;
      pendingNew = 0;
      chip.hidden = true;
    }
  }
  renderers.push(renderTimeline);

  function wireTimelineScroll() {
    const list = document.getElementById("timeline-list");
    const chip = document.getElementById("timeline-new-chip");
    list.addEventListener("scroll", () => {
      const atBottom = list.scrollTop + list.clientHeight >= list.scrollHeight - 4;
      timelinePinned = !atBottom;
      if (atBottom) { chip.hidden = true; pendingNew = 0; }
    });
    chip.addEventListener("click", () => {
      list.scrollTop = list.scrollHeight;
      chip.hidden = true;
      pendingNew = 0;
    });
  }

  // ── Fuzzer ──────────────────────────────────────────────────
  function renderFuzzer(_data, fuzz) {
    const seedEl = document.getElementById("fuzz-seed-inline");
    if (!fuzz) {
      seedEl.textContent = "—";
      document.getElementById("fuzz-kpis").innerHTML = "";
      document.querySelector("#fuzz-by-layer tbody").innerHTML = "";
      return;
    }
    seedEl.textContent = `#${fuzz.current_seed ?? "—"}`;
    const kpis = [
      { lbl: "Submitted", num: fuzz.txs_submitted_total ?? "—" },
      { lbl: "Applied", num: fuzz.txs_applied_total ?? "—" },
      { lbl: "Divergences", num: fuzz.divergences_total ?? "—" },
      { lbl: "Crashes", num: fuzz.crashes_total ?? "—" },
      { lbl: "Seed", num: fuzz.current_seed ?? "—", mono: true },
    ];
    document.getElementById("fuzz-kpis").innerHTML = kpis.map((k) =>
      `<div class="kpi"><div class="kpi-lbl">${k.lbl}</div><div class="kpi-num ${k.mono ? "mono" : ""}">${k.num}</div></div>`
    ).join("");

    // The fuzz API exposes only a total crash count (no per-layer breakdown),
    // so when any crash exists we mark every divergent row — "the fuzzer is
    // in a crashed state, every layer is suspect" — rather than guess which.
    const layerEntries = Object.entries(fuzz.divergences_total_by_layer ?? {}).sort(([a], [b]) => a.localeCompare(b));
    const anyCrash = (fuzz.crashes_total ?? 0) > 0;
    document.querySelector("#fuzz-by-layer tbody").innerHTML = layerEntries.map(([layer, count]) =>
      `<tr class="${anyCrash ? "crashed" : ""}"><td>${layer}</td><td>${count}</td></tr>`
    ).join("");
  }
  renderers.push(renderFuzzer);

  // Update footer context to reflect active tab
  function updateFooterContext() {
    const tab = currentTab();
    const seqs = (latest.nodes || []).map((n) => n.validated_ledger?.seq).filter(Boolean);
    const maxSeq = seqs.length ? Math.max(...seqs) : null;
    const ctx = document.getElementById("footer-ctx");
    if (tab === "fuzzer" && latestFuzz) ctx.innerHTML = `· seed <span class="mono">#${latestFuzz.current_seed}</span>`;
    else if (maxSeq) ctx.innerHTML = `· ledger <span class="mono">${maxSeq.toLocaleString("en-US")}</span>`;
    else ctx.textContent = "";
  }

  // ── Drawer ──────────────────────────────────────────────────
  let drawerName = null;
  let drawerPoll = null;

  async function fetchLogs(name) {
    return MOCK
      ? fetchJSON("/fixtures/mock-logs.json")
      : fetchJSON(`/api/logs/${encodeURIComponent(name)}`);
  }

  function renderDrawer(data) {
    const stateEl = document.getElementById("drawer-state");
    const logsEl = document.getElementById("drawer-logs");

    if (data?.state) {
      const s = data.state;
      const seq = s.validated_ledger
        ? `validated #${s.validated_ledger.seq}`
        : s.closed_ledger ? `closed #${s.closed_ledger.seq}` : "—";
      stateEl.innerHTML =
        `<b>${s.server_state || s.status || "—"}</b> · ${seq} · peers ${s.peers ?? "—"} · ${s.build_version || ""}`;
    } else {
      stateEl.textContent = "No state available.";
    }

    const entries = (data?.logs || []).slice().reverse();
    if (!entries.length) {
      logsEl.innerHTML = `<div class="row" style="color:var(--mid)">No log entries yet.</div>`;
      return;
    }
    logsEl.innerHTML = entries.map((e) => {
      const t = (e.ts.split("T")[1] || e.ts).split(".")[0];
      const klass = e.level === "error" || e.level === "unreachable" ? "lvl-err"
        : (e.level === "proposing" || e.level === "validating") ? "lvl-ok" : "";
      return `<div class="row"><span class="ts">${t}</span><span class="${klass}">${e.level}</span><span>${e.message}</span></div>`;
    }).join("");
  }

  async function openDrawer(name) {
    drawerName = name;
    document.getElementById("drawer").hidden = false;
    document.getElementById("drawer-title").textContent = `${name} · logs`;
    for (const card of document.querySelectorAll(".node-card")) {
      card.classList.toggle("selected", card.dataset.name === name);
    }
    for (const c of document.querySelectorAll(".topo-node-circle")) {
      c.classList.remove("selected");
    }
    const g = document.querySelector(`.topo-node[data-name="${CSS.escape(name)}"] .topo-node-circle`);
    if (g) g.classList.add("selected");

    const refresh = async () => {
      try { renderDrawer(await fetchLogs(name)); } catch { renderDrawer(null); }
    };
    await refresh();
    if (drawerPoll) clearInterval(drawerPoll);
    drawerPoll = setInterval(refresh, 2000);
  }

  function closeDrawer() {
    drawerName = null;
    document.getElementById("drawer").hidden = true;
    if (drawerPoll) { clearInterval(drawerPoll); drawerPoll = null; }
    for (const card of document.querySelectorAll(".node-card")) card.classList.remove("selected");
    for (const c of document.querySelectorAll(".topo-node-circle")) c.classList.remove("selected");
  }

  function wireDrawerResize() {
    const drawer = document.getElementById("drawer");
    const handle = document.getElementById("drawer-resize");
    let startY = 0, startH = 0, dragging = false;
    handle.addEventListener("pointerdown", (e) => {
      dragging = true;
      startY = e.clientY;
      startH = drawer.getBoundingClientRect().height;
      handle.setPointerCapture(e.pointerId);
    });
    handle.addEventListener("pointermove", (e) => {
      if (!dragging) return;
      const newH = Math.max(120, Math.min(window.innerHeight * 0.7, startH + (startY - e.clientY)));
      drawer.style.height = `${newH}px`;
    });
    handle.addEventListener("pointerup", () => { dragging = false; });
  }

  // ── Init ────────────────────────────────────────────────────
  document.addEventListener("DOMContentLoaded", () => {
    for (const el of document.querySelectorAll(".tab")) {
      el.addEventListener("click", () => setTab(el.dataset.tab));
    }
    window.addEventListener("hashchange", () => setTab(currentTab()));
    setTab(currentTab());

    document.getElementById("drawer-close").addEventListener("click", closeDrawer);
    wireDrawerResize();
    wireTimelineScroll();

    connectSSE();
    pollOnce();
    setInterval(pollOnce, 5000);

    setInterval(() => {
      if (!lastUpdate) return;
      const age = Math.max(0, Math.round((Date.now() - lastUpdate) / 1000));
      document.getElementById("footer-age").textContent = `${age}s ago`;
    }, 1000);
  });
})();
