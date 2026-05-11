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

  // Placeholder until Task 8 implements it
  function openDrawer(name) { console.log("openDrawer", name); }

  // ── Init ────────────────────────────────────────────────────
  document.addEventListener("DOMContentLoaded", () => {
    for (const el of document.querySelectorAll(".tab")) {
      el.addEventListener("click", () => setTab(el.dataset.tab));
    }
    window.addEventListener("hashchange", () => setTab(currentTab()));
    setTab(currentTab());

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
