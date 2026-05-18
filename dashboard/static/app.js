(() => {
  "use strict";

  const VIEWS = ["timeline", "chains", "topology", "fuzzer", "block"];
  const NAV_VIEWS = ["timeline", "chains", "topology", "fuzzer"];
  const PANES = ["rail", "main", "inspector"];
  const MOCK = new URLSearchParams(location.search).get("mock") === "1";

  // ── Store ───────────────────────────────────────────────────
  function createStore(initial) {
    let s = initial;
    const subs = new Set();
    const notify = () => subs.forEach((fn) => { try { fn(s); } catch (e) { console.error(e); } });
    return {
      get: () => s,
      set: (patch) => { s = { ...s, ...patch }; notify(); },
      setUI: (patch) => { s = { ...s, ui: { ...s.ui, ...patch } }; notify(); },
      setFilter: (patch) => { s = { ...s, ui: { ...s.ui, filters: { ...s.ui.filters, ...patch } } }; notify(); },
      subscribe: (fn) => { subs.add(fn); fn(s); return () => subs.delete(fn); },
    };
  }

  const store = createStore({
    nodes: [],
    fuzz: null,
    logs: {},
    block: { seq: null, data: null, loading: false, error: null, source: null, from: "timeline" },
    lastUpdate: 0,
    pendingUpdates: 0,
    sync: "connecting",
    ui: {
      view: "timeline",
      selected: null,
      paused: false,
      activePane: "main",
      inspectorCollapsed: false,
      filters: {
        nodes: "",
        timeline: "",
        onlyDivergences: false,
        logs: "",
        logLevels: { ok: true, warn: true, err: true },
        followTail: true,
      },
    },
  });

  // ── URL ↔ state ─────────────────────────────────────────────
  function parseHash() {
    const raw = location.hash.replace(/^#\/?/, "");
    const [path, query] = raw.split("?");
    const view = VIEWS.includes(path) ? path : "timeline";
    const params = new URLSearchParams(query || "");
    const seqRaw = params.get("seq");
    const blockSeq = seqRaw && /^\d+$/.test(seqRaw) ? Number(seqRaw) : null;
    return {
      view,
      selected: params.get("node") || null,
      onlyDivergences: params.get("only") === "diverged",
      timelineQuery: params.get("q") || "",
      blockSeq,
    };
  }

  function pushHash() {
    const s = store.get();
    const params = new URLSearchParams();
    if (s.ui.selected) params.set("node", s.ui.selected);
    if (s.ui.filters.onlyDivergences) params.set("only", "diverged");
    if (s.ui.filters.timeline) params.set("q", s.ui.filters.timeline);
    if (s.ui.view === "block" && s.block.seq != null) params.set("seq", String(s.block.seq));
    const q = params.toString();
    const target = `#/${s.ui.view}${q ? `?${q}` : ""}`;
    if (location.hash !== target) history.replaceState(null, "", target);
  }

  function applyHashToStore() {
    const h = parseHash();
    store.setUI({ view: h.view, selected: h.selected });
    store.setFilter({ onlyDivergences: h.onlyDivergences, timeline: h.timelineQuery });
    if (h.view === "block" && h.blockSeq != null && store.get().block.seq !== h.blockSeq) {
      loadBlock(h.blockSeq);
    }
  }

  // ── Mutators ────────────────────────────────────────────────
  function setView(view) {
    if (!VIEWS.includes(view)) view = "timeline";
    // Leaving the block sub-route via the main nav clears the loaded block
    // so the URL doesn't carry a stale `seq` while showing another view.
    if (view !== "block" && store.get().ui.view === "block") {
      store.set({ block: { seq: null, data: null, loading: false, error: null, source: null } });
    }
    store.setUI({ view });
    pushHash();
  }

  function setSelected(name) {
    store.setUI({ selected: name || null });
    pushHash();
  }

  function setPaused(paused) {
    store.setUI({ paused });
    if (!paused) store.set({ pendingUpdates: 0 });
  }

  function setActivePane(pane) {
    if (!PANES.includes(pane)) return;
    if (store.get().ui.activePane === pane) return;
    store.setUI({ activePane: pane });
  }

  async function loadBlock(seq) {
    seq = Number(seq);
    if (!Number.isInteger(seq) || seq <= 0) return;
    const current = store.get();
    const from = current.ui.view === "block" ? current.block.from : (NAV_VIEWS.includes(current.ui.view) ? current.ui.view : "timeline");
    store.set({ block: { seq, data: null, loading: true, error: null, source: null, from } });
    store.setUI({ view: "block" });
    pushHash();
    try {
      const url = MOCK ? "/fixtures/mock-ledger.json" : `/api/ledger/${seq}`;
      const data = await fetchJSON(url);
      if (data && data.result) {
        store.set({ block: { seq, data: data.result, loading: false, error: null, source: data.source || null, from } });
      } else {
        store.set({ block: { seq, data: null, loading: false, error: (data && data.error) || "Unknown response", source: null, from } });
      }
    } catch (e) {
      store.set({ block: { seq, data: null, loading: false, error: e.message, source: null, from } });
    }
  }

  function exitBlock() {
    const from = store.get().block.from || "timeline";
    store.set({ block: { seq: null, data: null, loading: false, error: null, source: null, from: "timeline" } });
    store.setUI({ view: from });
    pushHash();
  }

  // ── Data layer ──────────────────────────────────────────────
  async function fetchJSON(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`${url} → ${res.status}`);
    return res.json();
  }

  function setSync(state) {
    store.set({ sync: state });
  }

  function applyNodes(nodes) {
    if (store.get().ui.paused) {
      store.set({ pendingUpdates: store.get().pendingUpdates + 1, lastUpdate: Date.now() });
      return;
    }
    store.set({ nodes, lastUpdate: Date.now() });
  }

  function applyFuzz(fuzz) {
    if (store.get().ui.paused) return;
    store.set({ fuzz });
  }

  async function pollOnce() {
    if (MOCK) {
      try {
        applyNodes((await fetchJSON("/fixtures/mock.json")).nodes || []);
        applyFuzz(await fetchJSON("/fixtures/mock-fuzz.json"));
        setSync("connected");
      } catch (e) {
        console.error(e); setSync("offline");
      }
      return;
    }
    try {
      const data = await fetchJSON("/api/nodes");
      applyNodes(data.nodes || []);
      setSync("connected");
    } catch { setSync("reconnecting"); }
    try { applyFuzz(await fetchJSON("/api/fuzz")); } catch { applyFuzz(null); }
  }

  function connectSSE() {
    if (MOCK) return;
    const es = new EventSource("/events");
    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data);
        applyNodes(data.nodes || []);
        setSync("connected");
      } catch {}
    };
    es.onerror = () => { setSync("offline"); es.close(); setTimeout(connectSSE, 3000); };
  }

  // ── Sync pill subscriber ────────────────────────────────────
  store.subscribe((s) => {
    const badge = document.getElementById("sync-badge");
    const label = document.getElementById("sync-label");
    if (!badge || !label) return;
    badge.classList.remove("connected", "reconnecting", "offline", "connecting");
    badge.classList.add(s.sync);
    label.textContent = s.sync.toUpperCase();
  });

  // ── View switcher subscriber ────────────────────────────────
  store.subscribe((s) => {
    // The Block view is a sub-route — keep its parent tab lit while it's open
    // so the nav still tells the user where they came from.
    const navView = s.ui.view === "block" ? (s.block.from || "timeline") : s.ui.view;
    for (const seg of document.querySelectorAll(".seg")) {
      seg.classList.toggle("active", seg.dataset.view === navView);
    }
    for (const v of document.querySelectorAll(".view")) {
      v.classList.toggle("active", v.dataset.view === s.ui.view);
    }
  });

  // ── Topbar context subscriber ───────────────────────────────
  store.subscribe((s) => {
    const ctx = document.getElementById("topbar-ctx");
    if (!ctx) return;
    const seqs = (s.nodes || []).map((n) => n.validated_ledger?.seq).filter(Boolean);
    const maxSeq = seqs.length ? Math.max(...seqs) : null;
    const age = s.lastUpdate ? Math.max(0, Math.round((Date.now() - s.lastUpdate) / 1000)) : null;
    const parts = [];
    if (s.ui.view === "fuzzer" && s.fuzz) parts.push(`seed #${s.fuzz.current_seed}`);
    else if (s.ui.view === "block" && s.block.seq != null) parts.push(`block #${s.block.seq.toLocaleString("en-US")}`);
    else if (maxSeq) parts.push(`seq ${maxSeq.toLocaleString("en-US")}`);
    if (age != null) parts.push(`${age}s ago`);
    ctx.textContent = parts.length ? `· ${parts.join(" · ")}` : "—";
  });

  // ── Pause/pending subscriber ────────────────────────────────
  store.subscribe((s) => {
    const btn = document.getElementById("pause-btn");
    const chip = document.getElementById("pending-chip");
    const cnt = document.getElementById("pending-count");
    if (!btn || !chip || !cnt) return;
    btn.setAttribute("aria-pressed", s.ui.paused ? "true" : "false");
    btn.textContent = s.ui.paused ? "▶" : "⏸";
    if (s.ui.paused && s.pendingUpdates > 0) {
      chip.hidden = false;
      cnt.textContent = String(s.pendingUpdates);
    } else {
      chip.hidden = true;
    }
  });

  // ── Health classification ───────────────────────────────────
  const HEALTHY = new Set(["full", "proposing", "validating"]);
  function healthClass(n) {
    if (n.status !== "ok") return "health-err";
    return HEALTHY.has(n.server_state) ? "health-ok" : "health-warn";
  }

  // ── Rail: node list ─────────────────────────────────────────
  store.subscribe((s) => {
    const list = document.getElementById("node-list");
    if (!list) return;
    const q = s.ui.filters.nodes.trim().toLowerCase();
    const filtered = (s.nodes || []).filter((n) => !q || n.name.toLowerCase().includes(q));
    list.innerHTML = filtered.map((n) => `
      <div class="node-row ${s.ui.selected === n.name ? "selected" : ""}" data-name="${n.name}">
        <span class="node-pip ${healthClass(n)}"></span>
        <span class="node-name">${n.name}</span>
      </div>
    `).join("");
    if (!list.dataset.delegated) {
      list.addEventListener("click", (e) => {
        const row = e.target.closest(".node-row");
        if (row && row.dataset.name) setSelected(row.dataset.name);
      });
      list.dataset.delegated = "1";
    }
  });

  // ── Rail: KPIs ──────────────────────────────────────────────
  store.subscribe((s) => {
    const el = document.getElementById("rail-kpis");
    if (!el) return;
    const nodes = s.nodes || [];
    const ok = nodes.filter((n) => n.status === "ok");
    const seqs = ok.map((n) => n.validated_ledger?.seq ?? n.closed_ledger?.seq ?? n.ledger_current_index).filter(Boolean);
    const maxSeq = seqs.length ? Math.max(...seqs) : null;
    const minSeq = seqs.length ? Math.min(...seqs) : null;
    const convergeArr = ok.map((n) => n.last_close?.converge_time_s).filter((v) => v != null);
    const converge = convergeArr.length ? `${(convergeArr.reduce((a, b) => a + b, 0) / convergeArr.length).toFixed(1)}s` : "—";
    const spread = maxSeq != null && minSeq != null ? maxSeq - minSeq : 0;
    const divergences = s.fuzz?.divergences_total ?? 0;

    const kpis = [
      { lbl: "Ledger",   num: maxSeq ? maxSeq.toLocaleString("en-US") : "—" },
      { lbl: "Converge", num: converge },
      { lbl: "Spread",   num: spread },
      { lbl: "Diverge",  num: divergences },
      { lbl: "Online",   num: `${ok.length} / ${nodes.length}` },
    ];
    el.innerHTML = kpis.map((k) => `<div class="kpi"><div class="kpi-lbl">${k.lbl}</div><div class="kpi-num">${k.num}</div></div>`).join("");
  });

  // ── Timeline state ──────────────────────────────────────────
  const MAX_TIMELINE = 80;
  const MAX_CHAIN = 25;
  const prevValidatedSeqs = {};
  const prevClosedSeqs = {};
  const closeRows = new Map(); // seq → { time, hash, byNode: Map }
  // Per-node validated history (newest first), used by the Chains view to
  // visualize live desync. Each entry: { seq, hash, time }.
  const nodeChains = {};
  // seq → Map(nodeName → hash). Lets the Chains view colour each cell
  // against the majority hash for that seq across the fleet.
  const seqHashByNode = new Map();
  let pendingNew = 0;
  let timelinePinned = false;

  function pushNodeChain(name, seq, hash, time) {
    const list = nodeChains[name] || (nodeChains[name] = []);
    if (list.length && list[0].seq === seq) {
      // Same tip — fill in a hash we didn't have yet.
      if (!list[0].hash && hash) list[0].hash = hash;
    } else if (!list.length || list[0].seq < seq) {
      list.unshift({ seq, hash: hash || "", time });
      if (list.length > MAX_CHAIN) list.length = MAX_CHAIN;
    }
    if (hash) {
      const m = seqHashByNode.get(seq) || new Map();
      m.set(name, hash);
      seqHashByNode.set(seq, m);
    }
  }

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
        pushNodeChain(n.name, v, n.validated_ledger.hash || "", now);
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
      while (seqs.length && closeRows.size > MAX_TIMELINE) closeRows.delete(seqs.shift());
    }
  }

  function rowDiverged(row) {
    for (const v of row.byNode.values()) if (v === "diverged") return true;
    return false;
  }

  function divergenceCount() {
    let n = 0;
    for (const r of closeRows.values()) if (rowDiverged(r)) n += 1;
    return n;
  }

  // ── Timeline render ─────────────────────────────────────────
  store.subscribe((s) => {
    pushTimelineEvents(s.nodes || []);
    const list = document.getElementById("timeline-list");
    const chip = document.getElementById("timeline-new-chip");
    if (!list || !chip) return;

    const q = s.ui.filters.timeline.trim().toLowerCase();
    const onlyDiv = s.ui.filters.onlyDivergences;
    const nodeNames = (s.nodes || []).map((n) => n.name);
    const all = [...closeRows.entries()].sort(([a], [b]) => a - b);
    const visible = all.filter(([seq, row]) => {
      if (onlyDiv && !rowDiverged(row)) return false;
      if (!q) return true;
      return String(seq).includes(q) || (row.hash || "").toLowerCase().includes(q);
    });

    if (!visible.length) {
      list.innerHTML = `<div class="timeline-empty">${all.length ? "No matches." : "Waiting for ledger closes…"}</div>`;
      chip.hidden = true;
    } else {
      list.innerHTML = visible.map(([seq, row]) => {
        const time = row.time.toLocaleTimeString("en-US", { hour12: false });
        const hash = row.hash ? row.hash.slice(0, 12) + "…" : "";
        const dots = nodeNames.map((name) => {
          const status = row.byNode.get(name);
          const emph = s.ui.selected === name ? " emphasized" : "";
          if (!status) return `<span class="${emph}" style="background:var(--rule)" data-name="${name}"></span>`;
          return `<span class="${status === "diverged" ? "diverged" : ""}${emph}" data-name="${name}"></span>`;
        }).join("");
        const divClass = rowDiverged(row) ? "diverged" : "";
        return `<div class="timeline-row ${divClass}" data-seq="${seq}">
          <div class="timeline-seq" data-seq="${seq}" title="Open block details">${seq.toLocaleString("en-US")}</div>
          <div class="timeline-rule"></div>
          <div class="timeline-meta">
            <span>${time}</span>
            <span class="timeline-hash">${hash}</span>
            <span class="timeline-dots">${dots}</span>
          </div>
        </div>`;
      }).join("");

      // Wire per-dot click → select that node
      for (const dot of list.querySelectorAll(".timeline-dots span[data-name]")) {
        dot.addEventListener("click", (e) => { e.stopPropagation(); setSelected(dot.dataset.name); });
      }
      // Wire seq click → open block details
      for (const seqEl of list.querySelectorAll(".timeline-seq[data-seq]")) {
        seqEl.addEventListener("click", (e) => { e.stopPropagation(); loadBlock(Number(seqEl.dataset.seq)); });
      }

      if (timelinePinned && pendingNew > 0) {
        document.getElementById("timeline-new-count").textContent = pendingNew;
        chip.hidden = false;
      } else {
        list.scrollTop = list.scrollHeight;
        pendingNew = 0;
        chip.hidden = true;
      }
    }

    // Update Timeline segment badge
    const badge = document.getElementById("seg-timeline-badge");
    const n = divergenceCount();
    badge.textContent = n ? `${n} diverged` : "";
    badge.hidden = n === 0;
  });

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

  // ── Topology render ─────────────────────────────────────────
  store.subscribe((s) => {
    const svg = document.getElementById("topology-svg");
    if (!svg) return;
    const nodes = s.nodes || [];
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
      const sel = s.ui.selected === n.name ? "selected" : "";
      const cls = n.status !== "ok" ? "unreachable" : (HEALTHY.has(n.server_state) ? "" : "warn");
      html += `<g class="topo-node" data-name="${n.name}">
        <circle class="topo-node-circle ${cls} ${sel}" cx="${p.x}" cy="${p.y}" r="14"/>
        <text class="topo-node-label" x="${p.x}" y="${p.y + 22}">${n.name}</text>
      </g>`;
    }
    svg.innerHTML = html;
    for (const g of svg.querySelectorAll(".topo-node")) {
      g.addEventListener("click", () => setSelected(g.dataset.name));
    }
  });

  // ── Chains render (per-client block history) ───────────────
  // Strategy: for each node we keep up to MAX_CHAIN recent validated
  // ledgers. A cell is "diverged" when its hash differs from the most
  // common hash other nodes reported for the same seq. The tip cell is
  // marked "tip" so the column header's lag readout makes sense at a
  // glance. Clicking any cell jumps into the block details view.
  function majorityHashFor(seq) {
    const m = seqHashByNode.get(seq);
    if (!m || m.size === 0) return null;
    const counts = new Map();
    for (const h of m.values()) counts.set(h, (counts.get(h) || 0) + 1);
    let best = null, bestN = 0;
    for (const [h, n] of counts) {
      if (n > bestN) { best = h; bestN = n; }
    }
    return best;
  }

  store.subscribe((s) => {
    const grid = document.getElementById("chains-grid");
    const badge = document.getElementById("seg-chains-badge");
    if (!grid) return;
    const nodes = s.nodes || [];
    if (!nodes.length) {
      grid.innerHTML = `<div class="chain-empty">Waiting for nodes…</div>`;
      if (badge) { badge.hidden = true; badge.textContent = ""; }
      return;
    }

    const tips = nodes.map((n) => (nodeChains[n.name] || [])[0]?.seq || 0);
    const maxTip = tips.length ? Math.max(...tips) : 0;

    let divergedNodes = 0;
    const cols = nodes.map((n) => {
      const chain = nodeChains[n.name] || [];
      const tipSeq = chain[0]?.seq || 0;
      const lag = tipSeq && maxTip ? maxTip - tipSeq : 0;
      const health = healthClass(n);
      const sel = s.ui.selected === n.name ? "selected" : "";
      let colHasDiverge = false;
      const cells = chain.map((c, idx) => {
        const majority = majorityHashFor(c.seq);
        const diverged = c.hash && majority && c.hash !== majority;
        if (diverged) colHasDiverge = true;
        const cls = [diverged ? "diverged" : "", idx === 0 ? "tip" : ""].filter(Boolean).join(" ");
        const title = c.hash ? `${c.hash}` : "";
        return `<div class="chain-cell ${cls}" data-seq="${c.seq}" title="${escapeHTML(title)}">
          <span class="chain-seq">${c.seq.toLocaleString("en-US")}</span>
          <span class="chain-hash">${escapeHTML(shortHash(c.hash))}</span>
        </div>`;
      }).join("");
      if (colHasDiverge) divergedNodes += 1;
      const lagChip = lag > 0
        ? `<span class="chain-lag" title="behind leader by ${lag} ledger${lag === 1 ? "" : "s"}">−${lag}</span>`
        : tipSeq && tipSeq === maxTip ? `<span class="chain-tip-leader" title="at fleet leader tip">●</span>` : "";
      return `<div class="chain-col ${sel}" data-name="${escapeHTML(n.name)}">
        <header class="chain-head">
          <span class="node-pip ${health}"></span>
          <span class="chain-name">${escapeHTML(n.name)}</span>
          ${lagChip}
        </header>
        <div class="chain-cells">${cells || `<div class="chain-empty">Waiting for ledger close…</div>`}</div>
      </div>`;
    }).join("");

    grid.innerHTML = cols;
    if (!grid.dataset.delegated) {
      grid.addEventListener("click", (e) => {
        const cell = e.target.closest(".chain-cell[data-seq]");
        if (cell) { loadBlock(Number(cell.dataset.seq)); return; }
        const col = e.target.closest(".chain-col[data-name]");
        if (col) setSelected(col.dataset.name);
      });
      grid.dataset.delegated = "1";
    }

    if (badge) {
      if (divergedNodes > 0) {
        badge.textContent = `${divergedNodes} desync${divergedNodes === 1 ? "" : "ed"}`;
        badge.hidden = false;
      } else {
        badge.hidden = true;
        badge.textContent = "";
      }
    }
  });

  // ── Block (ledger) render ───────────────────────────────────
  function fmtAmount(amt) {
    if (amt == null) return "";
    if (typeof amt === "string") {
      // Drops → XRP (drops are integer strings)
      if (/^\d+$/.test(amt)) {
        const drops = BigInt(amt);
        const whole = drops / 1000000n;
        const frac = drops % 1000000n;
        const fracStr = frac.toString().padStart(6, "0").replace(/0+$/, "");
        return `${whole.toString()}${fracStr ? "." + fracStr : ""} XRP`;
      }
      return amt;
    }
    if (typeof amt === "object" && amt.value) {
      return `${amt.value} ${amt.currency || ""}`.trim() + (amt.issuer ? ` · ${amt.issuer.slice(0, 8)}…` : "");
    }
    return String(amt);
  }

  function escapeHTML(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  }

  function shortHash(h) {
    if (!h) return "";
    return h.length > 18 ? h.slice(0, 8) + "…" + h.slice(-6) : h;
  }

  // Best-effort close_time → ISO string. XRPL close_time is seconds since
  // the Ripple epoch (2000-01-01T00:00:00Z), 946684800 unix.
  function fmtCloseTime(ledger) {
    if (ledger.close_time_human) return ledger.close_time_human;
    if (typeof ledger.close_time === "number") {
      const unix = (ledger.close_time + 946684800) * 1000;
      return new Date(unix).toISOString().replace("T", " ").replace("Z", " UTC");
    }
    return "";
  }

  store.subscribe((s) => {
    if (s.ui.view !== "block") return;
    const seqEl = document.getElementById("block-seq");
    const hashEl = document.getElementById("block-hash");
    const empty = document.getElementById("block-empty");
    const fleet = document.getElementById("block-fleet");
    const meta = document.getElementById("block-meta");
    const txs = document.getElementById("block-txs");
    const source = document.getElementById("block-source");
    if (!seqEl) return;

    seqEl.textContent = s.block.seq != null ? `#${s.block.seq.toLocaleString("en-US")}` : "—";
    source.textContent = s.block.source ? `via ${s.block.source}` : "";

    if (s.block.loading) {
      empty.hidden = false;
      empty.textContent = "Loading…";
      hashEl.textContent = "—";
      fleet.hidden = true; meta.hidden = true; txs.hidden = true;
      return;
    }
    if (s.block.error) {
      empty.hidden = false;
      empty.textContent = `Failed to load ledger: ${s.block.error}`;
      hashEl.textContent = "—";
      fleet.hidden = true; meta.hidden = true; txs.hidden = true;
      return;
    }
    const result = s.block.data;
    if (!result || !(result.ledger || result.ledger_hash)) {
      empty.hidden = false;
      empty.textContent = "No ledger data.";
      hashEl.textContent = "—";
      fleet.hidden = true; meta.hidden = true; txs.hidden = true;
      return;
    }
    empty.hidden = true;

    const ledger = result.ledger || {};
    const ledgerHash = result.ledger_hash || ledger.ledger_hash || ledger.hash || "";
    hashEl.textContent = ledgerHash || "—";

    // Fleet agreement — uses local close-row data captured from the live stream
    const row = closeRows.get(s.block.seq);
    const nodeNames = (s.nodes || []).map((n) => n.name);
    const fleetBody = document.getElementById("block-fleet-body");
    if (row && nodeNames.length) {
      fleet.hidden = false;
      const localHash = row.hash || ledgerHash;
      fleetBody.innerHTML = nodeNames.map((name) => {
        const status = row.byNode.get(name);
        let label = "missing";
        let cls = "block-status-missing";
        if (status === "match") { label = "match"; cls = "block-status-match"; }
        else if (status === "diverged") { label = "diverged"; cls = "block-status-diverged"; }
        return `<tr class="${status === "diverged" ? "diverged" : ""}">
          <td>${escapeHTML(name)}</td>
          <td>${escapeHTML(shortHash(localHash))}</td>
          <td class="${cls}">${label}</td>
        </tr>`;
      }).join("");
    } else {
      fleet.hidden = true;
    }

    // Ledger header
    const metaPairs = [
      ["Ledger index", ledger.ledger_index || result.ledger_index || s.block.seq],
      ["Ledger hash", ledgerHash],
      ["Parent hash", ledger.parent_hash],
      ["Account state hash", ledger.account_hash],
      ["Transaction hash", ledger.transaction_hash],
      ["Close time", fmtCloseTime(ledger)],
      ["Close flags", ledger.close_flags != null ? String(ledger.close_flags) : ""],
      ["Close time resolution", ledger.close_time_resolution != null ? String(ledger.close_time_resolution) : ""],
      ["Parent close time", ledger.parent_close_time != null ? String(ledger.parent_close_time) : ""],
      ["Total coins", ledger.total_coins ? fmtAmount(ledger.total_coins) : ""],
      ["Validated", String(Boolean(result.validated))],
    ].filter(([, v]) => v !== undefined && v !== null && v !== "");
    document.getElementById("block-meta-list").innerHTML = metaPairs.map(
      ([k, v]) => `<dt>${escapeHTML(k)}</dt><dd>${escapeHTML(v)}</dd>`
    ).join("");
    meta.hidden = false;

    // Transactions
    const txList = Array.isArray(ledger.transactions) ? ledger.transactions : [];
    document.getElementById("block-tx-count").textContent = txList.length;
    const txEmpty = document.getElementById("block-tx-empty");
    const txTable = document.getElementById("block-tx-table");
    const txBody = document.getElementById("block-tx-body");
    if (!txList.length) {
      txEmpty.hidden = false;
      txTable.hidden = true;
    } else {
      txEmpty.hidden = true;
      txTable.hidden = false;
      txBody.innerHTML = txList.map((tx, i) => {
        // With expand:true, each entry can be an object with the tx fields
        // (and optionally a `metaData` sibling), or a bare hash string when
        // the node doesn't honor expand.
        const t = typeof tx === "string" ? { hash: tx } : tx;
        const hash = t.hash || t.tx_hash || (t.metaData && t.metaData.TransactionResult ? "" : "") || "";
        return `<tr>
          <td>${i + 1}</td>
          <td>${escapeHTML(t.TransactionType || "—")}</td>
          <td>${escapeHTML(t.Account || "")}</td>
          <td>${escapeHTML(t.Destination || "")}</td>
          <td>${escapeHTML(fmtAmount(t.Amount || t.DeliverMax))}</td>
          <td title="${escapeHTML(hash)}">${escapeHTML(shortHash(hash))}</td>
        </tr>`;
      }).join("");
    }
    txs.hidden = false;
  });

  // ── Fuzzer render ───────────────────────────────────────────
  let fuzzSortDir = "desc"; // toggles when user clicks the Divergences header
  store.subscribe((s) => {
    const seedEl = document.getElementById("fuzz-seed-inline");
    if (!seedEl) return;
    const fuzz = s.fuzz;
    if (!fuzz) {
      seedEl.textContent = "—";
      document.getElementById("fuzz-kpis").innerHTML = "";
      document.querySelector("#fuzz-by-layer tbody").innerHTML = "";
      const badge = document.getElementById("seg-fuzzer-badge");
      badge.hidden = true;
      return;
    }
    seedEl.textContent = `#${fuzz.current_seed ?? "—"}`;
    const kpis = [
      { lbl: "Submitted",   num: fuzz.txs_submitted_total ?? "—" },
      { lbl: "Applied",     num: fuzz.txs_applied_total ?? "—" },
      { lbl: "Divergences", num: fuzz.divergences_total ?? "—" },
      { lbl: "Crashes",     num: fuzz.crashes_total ?? "—" },
      { lbl: "Seed",        num: fuzz.current_seed ?? "—", mono: true },
    ];
    document.getElementById("fuzz-kpis").innerHTML = kpis.map((k) =>
      `<div class="kpi"><div class="kpi-lbl">${k.lbl}</div><div class="kpi-num ${k.mono ? "mono" : ""}">${k.num}</div></div>`
    ).join("");

    const entries = Object.entries(fuzz.divergences_total_by_layer ?? {});
    entries.sort(([la, ca], [lb, cb]) => fuzzSortDir === "desc" ? cb - ca : ca - cb);
    const anyCrash = (fuzz.crashes_total ?? 0) > 0;
    document.querySelector("#fuzz-by-layer tbody").innerHTML = entries.map(([layer, count]) =>
      `<tr class="${anyCrash ? "crashed" : ""}"><td>${layer}</td><td>${count}</td></tr>`
    ).join("");

    const badge = document.getElementById("seg-fuzzer-badge");
    const crashes = fuzz.crashes_total ?? 0;
    badge.textContent = crashes ? `${crashes} crash${crashes === 1 ? "" : "es"}` : "";
    badge.hidden = crashes === 0;
  });

  // ── Inspector: log polling ──────────────────────────────────
  let inspectorPoll = null;
  async function fetchLogs(name) {
    return MOCK
      ? fetchJSON("/fixtures/mock-logs.json")
      : fetchJSON(`/api/logs/${encodeURIComponent(name)}`);
  }

  async function refreshLogsFor(name) {
    if (!name) return;
    try {
      const data = await fetchLogs(name);
      store.set({ logs: { ...store.get().logs, [name]: data } });
    } catch {
      store.set({ logs: { ...store.get().logs, [name]: null } });
    }
  }

  function startInspectorPolling(name) {
    if (inspectorPoll) clearInterval(inspectorPoll);
    refreshLogsFor(name);
    inspectorPoll = setInterval(() => refreshLogsFor(name), 2000);
  }

  function stopInspectorPolling() {
    if (inspectorPoll) { clearInterval(inspectorPoll); inspectorPoll = null; }
  }

  // ── Inspector: empty-state (fleet summary) ──────────────────
  function renderFleetSummary(s) {
    const nodes = s.nodes || [];
    const ok = nodes.filter((n) => n.status === "ok");
    const seqs = ok.map((n) => n.validated_ledger?.seq ?? n.closed_ledger?.seq ?? n.ledger_current_index).filter(Boolean);
    const maxSeq = seqs.length ? Math.max(...seqs) : null;
    const minSeq = seqs.length ? Math.min(...seqs) : null;
    const spread = maxSeq != null && minSeq != null ? maxSeq - minSeq : 0;
    const divergences = s.fuzz?.divergences_total ?? 0;
    const crashes = s.fuzz?.crashes_total ?? 0;
    const rows = [
      { item: "Ledger spread", status: spread <= 1 ? "synced" : `${spread} ledgers apart`, count: spread },
      { item: "Unreachable nodes", status: nodes.length - ok.length === 0 ? "none" : "needs attention", count: nodes.length - ok.length },
      { item: "Fuzzer divergences", status: divergences === 0 ? "clean" : "investigating", count: divergences },
      { item: "Fuzzer crashes", status: crashes === 0 ? "clean" : "open", count: crashes },
    ];
    document.querySelector("#overview-summary tbody").innerHTML = rows
      .map((r) => `<tr><td>${r.item}</td><td>${r.status}</td><td>${r.count}</td></tr>`).join("");
  }

  // ── Inspector: state line + logs ────────────────────────────
  function levelOf(e) {
    if (e.level === "error" || e.level === "unreachable") return "err";
    if (e.level === "proposing" || e.level === "validating") return "ok";
    return "warn";
  }

  function renderInspectorState(s, data) {
    const stateEl = document.getElementById("inspector-state");
    if (data?.state) {
      const ss = data.state;
      const seq = ss.validated_ledger ? `validated #${ss.validated_ledger.seq}`
                : ss.closed_ledger ? `closed #${ss.closed_ledger.seq}` : "—";
      const proposers = ss.last_close?.proposers ?? "—";
      stateEl.innerHTML = `<b>${ss.server_state || ss.status || "—"}</b> · ${seq} · peers ${ss.peers ?? "—"} · proposers ${proposers} · ${ss.build_version || ""}`;
    } else if (data === null) {
      stateEl.textContent = "Failed to fetch state.";
    } else {
      stateEl.textContent = "Loading…";
    }
  }

  let inspectorPendingScroll = false;
  function renderInspectorLogs(s, data) {
    const logsEl = document.getElementById("inspector-logs");
    const f = s.ui.filters;
    const wasAtBottom = logsEl.scrollHeight - logsEl.scrollTop - logsEl.clientHeight < 8;
    let entries = (data?.logs || []).slice().reverse();
    entries = entries.filter((e) => f.logLevels[levelOf(e)]);
    const q = f.logs.trim().toLowerCase();
    if (q) entries = entries.filter((e) => (e.message || "").toLowerCase().includes(q) || (e.level || "").toLowerCase().includes(q));
    if (data === undefined) {
      logsEl.innerHTML = `<div class="row" style="color:var(--mid)">Loading…</div>`;
    } else if (!entries.length) {
      logsEl.innerHTML = `<div class="row" style="color:var(--mid)">No log entries.</div>`;
    } else {
      logsEl.innerHTML = entries.map((e) => {
        const t = (e.ts.split("T")[1] || e.ts).split(".")[0];
        const klass = `lvl-${levelOf(e)}`;
        return `<div class="row"><span class="ts">${t}</span><span class="${klass}">${e.level}</span><span>${e.message}</span></div>`;
      }).join("");
    }
    if (entries.length && (inspectorPendingScroll || (f.followTail && wasAtBottom))) {
      logsEl.scrollTop = logsEl.scrollHeight;
      inspectorPendingScroll = false;
    }
  }

  // ── Inspector subscriber ────────────────────────────────────
  let prevSelected = null;
  store.subscribe((s) => {
    const title = document.getElementById("inspector-title");
    const clear = document.getElementById("inspector-clear");
    const empty = document.getElementById("inspector-empty");
    const stateEl = document.getElementById("inspector-state");
    const logsHead = document.getElementById("inspector-logs-head");
    const logsEl = document.getElementById("inspector-logs");
    if (!title) return;

    if (s.ui.selected !== prevSelected) {
      prevSelected = s.ui.selected;
      if (s.ui.selected) {
        inspectorPendingScroll = true;
        startInspectorPolling(s.ui.selected);
      } else stopInspectorPolling();
    }

    const inspector = document.getElementById("inspector");
    inspector.classList.toggle("open", !!s.ui.selected);

    if (!s.ui.selected) {
      title.textContent = "Fleet";
      clear.hidden = true;
      stateEl.textContent = "";
      empty.hidden = false;
      logsHead.hidden = true;
      logsEl.hidden = true;
      renderFleetSummary(s);
      return;
    }

    title.textContent = s.ui.selected;
    clear.hidden = false;
    empty.hidden = true;
    logsHead.hidden = false;
    logsEl.hidden = false;

    const data = s.logs[s.ui.selected];
    renderInspectorState(s, data);
    renderInspectorLogs(s, data);
  });

  // ── Keyboard ────────────────────────────────────────────────
  function isTypingTarget(t) {
    return t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);
  }

  function focusFilterForActivePane() {
    const pane = store.get().ui.activePane;
    const id = pane === "rail" ? "rail-filter" : pane === "main" && store.get().ui.view === "timeline" ? "timeline-filter" : pane === "inspector" ? "log-filter" : null;
    if (id) document.getElementById(id).focus();
  }

  let railFocusIdx = -1;
  let timelineFocusIdx = -1;
  let logFocusIdx = -1;

  function refreshKbFocus() {
    document.querySelectorAll(".node-row.kb-focus, .timeline-row.kb-focus, .inspector-logs .row.kb-focus").forEach((el) => el.classList.remove("kb-focus"));
    if (store.get().ui.activePane === "rail" && railFocusIdx >= 0) {
      const rows = document.querySelectorAll(".node-row");
      rows[railFocusIdx]?.classList.add("kb-focus");
      rows[railFocusIdx]?.scrollIntoView({ block: "nearest" });
    }
    if (store.get().ui.activePane === "main" && store.get().ui.view === "timeline" && timelineFocusIdx >= 0) {
      const rows = document.querySelectorAll(".timeline-row");
      rows[timelineFocusIdx]?.classList.add("kb-focus");
      rows[timelineFocusIdx]?.scrollIntoView({ block: "nearest" });
    }
    if (store.get().ui.activePane === "inspector" && logFocusIdx >= 0) {
      const rows = document.querySelectorAll(".inspector-logs .row");
      rows[logFocusIdx]?.classList.add("kb-focus");
      rows[logFocusIdx]?.scrollIntoView({ block: "nearest" });
    }
  }

  function walkList(delta) {
    const pane = store.get().ui.activePane;
    if (pane === "rail") {
      const rows = document.querySelectorAll(".node-row");
      if (!rows.length) return;
      railFocusIdx = Math.max(0, Math.min(rows.length - 1, (railFocusIdx < 0 ? 0 : railFocusIdx + delta)));
    } else if (pane === "main" && store.get().ui.view === "timeline") {
      const rows = document.querySelectorAll(".timeline-row");
      if (!rows.length) return;
      timelineFocusIdx = Math.max(0, Math.min(rows.length - 1, (timelineFocusIdx < 0 ? 0 : timelineFocusIdx + delta)));
    } else if (pane === "inspector") {
      const rows = document.querySelectorAll(".inspector-logs .row");
      if (!rows.length) return;
      logFocusIdx = Math.max(0, Math.min(rows.length - 1, (logFocusIdx < 0 ? 0 : logFocusIdx + delta)));
    }
    refreshKbFocus();
  }

  function activateFocused() {
    const pane = store.get().ui.activePane;
    if (pane === "rail") {
      const rows = document.querySelectorAll(".node-row");
      const r = rows[railFocusIdx];
      if (r) setSelected(r.dataset.name);
    } else if (pane === "main" && store.get().ui.view === "timeline") {
      const rows = document.querySelectorAll(".timeline-row");
      const r = rows[timelineFocusIdx];
      if (r && r.dataset.seq) loadBlock(Number(r.dataset.seq));
    } else if (pane === "inspector") {
      const cb = document.getElementById("log-follow");
      cb.checked = !cb.checked;
      store.setFilter({ followTail: cb.checked });
    }
  }

  function jumpNextDivergence() {
    if (store.get().ui.view !== "timeline") return;
    const rows = [...document.querySelectorAll(".timeline-row.diverged")];
    if (!rows.length) return;
    const current = rows.findIndex((r) => r.classList.contains("kb-focus"));
    const next = rows[(current + 1) % rows.length];
    next.scrollIntoView({ block: "center" });
    rows.forEach((r) => r.classList.remove("kb-focus"));
    next.classList.add("kb-focus");
  }

  function copyFocused() {
    const pane = store.get().ui.activePane;
    let text = null;
    if (pane === "inspector") {
      const focused = document.querySelector(".inspector-logs .row.kb-focus");
      text = focused ? focused.innerText : document.getElementById("inspector-state").innerText;
    } else if (pane === "main" && store.get().ui.view === "timeline") {
      const focused = document.querySelector(".timeline-row.kb-focus");
      if (focused) text = focused.innerText;
    }
    if (text) navigator.clipboard?.writeText(text).catch(() => {});
  }

  function cyclePane(delta) {
    const order = PANES;
    const i = order.indexOf(store.get().ui.activePane);
    setActivePane(order[(i + delta + order.length) % order.length]);
  }

  document.addEventListener("keydown", (e) => {
    // Cheatsheet / palette open while typing: esc should still close
    if (e.key === "Escape") {
      const palette = document.getElementById("palette");
      if (!palette.hidden) { palette.hidden = true; return; }
      const cheats = document.getElementById("cheatsheet");
      if (!cheats.hidden) { cheats.hidden = true; return; }
      if (isTypingTarget(e.target)) {
        e.target.blur();
        // Clear that input's value via store
        if (e.target.id === "rail-filter") { store.setFilter({ nodes: "" }); e.target.value = ""; }
        if (e.target.id === "timeline-filter") { store.setFilter({ timeline: "" }); e.target.value = ""; pushHash(); }
        if (e.target.id === "log-filter") { store.setFilter({ logs: "" }); e.target.value = ""; }
        return;
      }
      if (store.get().ui.view === "block") { exitBlock(); return; }
      if (store.get().ui.selected) { setSelected(null); return; }
      return;
    }
    if (isTypingTarget(e.target)) return;
    if (e.metaKey && e.key.toLowerCase() === "k") { e.preventDefault(); document.getElementById("palette").hidden = false; document.getElementById("palette-input").focus(); return; }
    if (e.ctrlKey && e.key.toLowerCase() === "k") { e.preventDefault(); document.getElementById("palette").hidden = false; document.getElementById("palette-input").focus(); return; }

    switch (e.key) {
      case "1": setView("timeline"); break;
      case "2": setView("chains"); break;
      case "3": setView("topology"); break;
      case "4": setView("fuzzer"); break;
      case "j": case "J": walkList(+1); break;
      case "k": case "K": walkList(-1); break;
      case "Enter": activateFocused(); break;
      case "/": e.preventDefault(); focusFilterForActivePane(); break;
      case " ": e.preventDefault(); setPaused(!store.get().ui.paused); break;
      case "n": case "N": jumpNextDivergence(); break;
      case "c": case "C": copyFocused(); break;
      case "e": case "E": document.body.classList.toggle("wide-inspector"); break;
      case "?": document.getElementById("cheatsheet").hidden = false; break;
      case "Tab":
        e.preventDefault();
        cyclePane(e.shiftKey ? -1 : +1);
        break;
    }
  });

  // Cheatsheet dismiss (click outside the card)
  document.addEventListener("DOMContentLoaded", () => {
    document.getElementById("cheatsheet").addEventListener("click", (e) => {
      if (e.currentTarget === e.target) e.currentTarget.hidden = true;
    });
  });

  // Pane focus visual
  store.subscribe((s) => {
    document.querySelectorAll("[data-pane]").forEach((el) => el.classList.toggle("pane-focus", el.dataset.pane === s.ui.activePane));
  });

  // ── Command palette ─────────────────────────────────────────
  let paletteFocus = 0;

  function paletteEntries() {
    const s = store.get();
    const out = [];
    for (const n of (s.nodes || [])) out.push({ label: `Node: ${n.name}`, run: () => setSelected(n.name) });
    for (const v of VIEWS) out.push({ label: `View: ${v}`, run: () => setView(v) });
    out.push({ label: s.ui.paused ? "Resume" : "Pause", run: () => setPaused(!s.ui.paused) });
    out.push({ label: "Copy state", run: () => {
      const t = document.getElementById("inspector-state").innerText;
      if (t) navigator.clipboard?.writeText(t).catch(() => {});
    }});
    return out;
  }

  function renderPalette(filter) {
    const list = document.getElementById("palette-list");
    const q = filter.trim().toLowerCase();
    const items = paletteEntries().filter((e) => !q || e.label.toLowerCase().includes(q));
    paletteFocus = items.length ? Math.max(0, Math.min(paletteFocus, items.length - 1)) : 0;
    list.innerHTML = items.map((e, i) => `<li class="${i === paletteFocus ? "kb-focus" : ""}" data-i="${i}">${e.label}</li>`).join("");
    for (const li of list.querySelectorAll("li")) {
      li.addEventListener("click", () => { items[Number(li.dataset.i)].run(); closePalette(); });
    }
    return items;
  }

  function closePalette() {
    document.getElementById("palette").hidden = true;
    document.getElementById("palette-input").value = "";
    paletteFocus = 0;
  }

  document.addEventListener("DOMContentLoaded", () => {
    const palette = document.getElementById("palette");
    const input = document.getElementById("palette-input");
    palette.addEventListener("click", (e) => { if (e.target === palette) closePalette(); });
    input.addEventListener("input", () => renderPalette(input.value));
    input.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown") { e.preventDefault(); paletteFocus += 1; renderPalette(input.value); }
      else if (e.key === "ArrowUp") { e.preventDefault(); paletteFocus = Math.max(0, paletteFocus - 1); renderPalette(input.value); }
      else if (e.key === "Enter") {
        const items = renderPalette(input.value);
        if (items[paletteFocus]) { items[paletteFocus].run(); closePalette(); }
      } else if (e.key === "Escape") { closePalette(); }
    });
    document.getElementById("palette-btn").addEventListener("click", () => {
      palette.hidden = false;
      renderPalette("");
      input.focus();
    });
  });

  // ── Init ────────────────────────────────────────────────────
  document.addEventListener("DOMContentLoaded", () => {
    // Wire main switch
    for (const seg of document.querySelectorAll(".seg")) {
      seg.addEventListener("click", () => setView(seg.dataset.view));
    }
    // Wire pause
    document.getElementById("pause-btn").addEventListener("click", () => setPaused(!store.get().ui.paused));
    // Active pane on click (must be `click`, not `mousedown` — mousedown would
    // run before the row's own click handler and re-render the rail mid-press,
    // dropping the click).
    for (const el of document.querySelectorAll("[data-pane]")) {
      el.addEventListener("click", () => setActivePane(el.dataset.pane));
    }
    // Rail filter
    document.getElementById("rail-filter").addEventListener("input", (e) => {
      store.setFilter({ nodes: e.target.value });
    });
    // Timeline filter inputs
    document.getElementById("timeline-filter").addEventListener("input", (e) => {
      store.setFilter({ timeline: e.target.value });
      pushHash();
    });
    document.getElementById("timeline-only-diverged").addEventListener("change", (e) => {
      store.setFilter({ onlyDivergences: e.target.checked });
      pushHash();
    });
    wireTimelineScroll();

    // Fuzzer layer sort
    document.querySelectorAll("#fuzz-by-layer thead th").forEach((th, i) => {
      if (i !== 1) return; // only Divergences col toggles
      th.addEventListener("click", () => {
        fuzzSortDir = fuzzSortDir === "desc" ? "asc" : "desc";
        store.set({}); // trigger re-render
      });
    });
    // Block view back button
    document.getElementById("block-back").addEventListener("click", () => exitBlock());

    // Inspector clear
    document.getElementById("inspector-clear").addEventListener("click", () => setSelected(null));
    document.getElementById("inspector-expand").addEventListener("click", () => {
      document.body.classList.toggle("wide-inspector");
    });

    // Log filters
    document.getElementById("log-filter").addEventListener("input", (e) => {
      store.setFilter({ logs: e.target.value });
    });
    document.getElementById("log-follow").addEventListener("change", (e) => {
      store.setFilter({ followTail: e.target.checked });
    });
    for (const chip of document.querySelectorAll(".log-chip")) {
      chip.addEventListener("click", () => {
        const lv = chip.dataset.level;
        const current = store.get().ui.filters.logLevels;
        store.setFilter({ logLevels: { ...current, [lv]: !current[lv] } });
        chip.classList.toggle("active");
      });
    }

    // Hash routing
    applyHashToStore();
    window.addEventListener("hashchange", applyHashToStore);

    // Reflect URL filters back into inputs on load (runs after applyHashToStore)
    const initFilters = store.get().ui.filters;
    document.getElementById("timeline-filter").value = initFilters.timeline;
    document.getElementById("timeline-only-diverged").checked = initFilters.onlyDivergences;

    // Data
    connectSSE();
    pollOnce();
    setInterval(pollOnce, 5000);

    // Tick the age indicator once a second
    setInterval(() => store.set({ /* no-op patch */ }), 1000);
  });

  // Expose for later tasks
  window.__wb = { store, setView, setSelected, setPaused, setActivePane, parseHash, pushHash };
})();
