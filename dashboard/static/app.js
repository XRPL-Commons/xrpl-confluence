(() => {
  "use strict";

  const VIEWS = ["timeline", "topology", "fuzzer"];
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
    return {
      view,
      selected: params.get("node") || null,
      onlyDivergences: params.get("only") === "diverged",
      timelineQuery: params.get("q") || "",
    };
  }

  function pushHash() {
    const s = store.get();
    const params = new URLSearchParams();
    if (s.ui.selected) params.set("node", s.ui.selected);
    if (s.ui.filters.onlyDivergences) params.set("only", "diverged");
    if (s.ui.filters.timeline) params.set("q", s.ui.filters.timeline);
    const q = params.toString();
    const target = `#/${s.ui.view}${q ? `?${q}` : ""}`;
    if (location.hash !== target) history.replaceState(null, "", target);
  }

  function applyHashToStore() {
    const h = parseHash();
    store.setUI({ view: h.view, selected: h.selected });
    store.setFilter({ onlyDivergences: h.onlyDivergences, timeline: h.timelineQuery });
  }

  // ── Mutators ────────────────────────────────────────────────
  function setView(view) {
    if (!VIEWS.includes(view)) view = "timeline";
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
    store.setUI({ activePane: pane });
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
    for (const seg of document.querySelectorAll(".seg")) {
      seg.classList.toggle("active", seg.dataset.view === s.ui.view);
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
    for (const row of list.querySelectorAll(".node-row")) {
      row.addEventListener("click", () => setSelected(row.dataset.name));
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

  // ── Init ────────────────────────────────────────────────────
  document.addEventListener("DOMContentLoaded", () => {
    // Wire main switch
    for (const seg of document.querySelectorAll(".seg")) {
      seg.addEventListener("click", () => setView(seg.dataset.view));
    }
    // Wire pause
    document.getElementById("pause-btn").addEventListener("click", () => setPaused(!store.get().ui.paused));
    // Active pane on click
    for (const el of document.querySelectorAll("[data-pane]")) {
      el.addEventListener("mousedown", () => setActivePane(el.dataset.pane));
    }
    // Rail filter
    document.getElementById("rail-filter").addEventListener("input", (e) => {
      store.setFilter({ nodes: e.target.value });
    });
    // Hash routing
    applyHashToStore();
    window.addEventListener("hashchange", applyHashToStore);

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
