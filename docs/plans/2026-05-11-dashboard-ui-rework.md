# Dashboard UI Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Confluence dashboard frontend as a single centered "white poster on dark canvas" with a tabbed editorial-serif layout, per `docs/design/2026-05-11-dashboard-ui-rework-design.md`.

**Architecture:** Frontend-only rewrite of three files (`index.html`, `style.css`, `app.js`) inside `dashboard/static/`. No framework, no build step, no backend changes. Existing data fetch logic (SSE `/events`, polling `/api/nodes`, `/api/logs/:name`, `/api/fuzz`) is preserved; only render targets and styling change. Tabs are hash-routed (`#overview`, `#nodes`, `#topology`, `#timeline`, `#fuzzer`). A shared in-card drawer component services both Nodes and Topology log inspection. A `?mock=1` query parameter loads a small fixture so visual work can be verified without spinning up Kurtosis.

**Tech Stack:** Vanilla HTML5, CSS3 (custom properties, grid, media queries, `@media print`), ES module-free JS (IIFE), web fonts via Google Fonts CDN (Inter, JetBrains Mono) + Georgia/serif fallback for display.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `dashboard/static/index.html` | Full rewrite | Page shell, tab structure, 5 empty tab-panel containers, footer line, font preloads |
| `dashboard/static/style.css` | Full rewrite | Design tokens, poster frame, tab bar, typography, per-tab layouts, drawer, responsive, print |
| `dashboard/static/app.js` | Restructure | Hash router, per-tab renderers, shared drawer, footer-tick, preserved data layer (SSE/poll/fuzz/logs) |
| `dashboard/static/fixtures/mock.json` | New | Static fixture used when URL has `?mock=1`. Mirrors `/api/nodes` shape with 5 nodes (3 rippled, 2 goxrpl). |
| `dashboard/static/fixtures/mock-fuzz.json` | New | Fixture mirroring `/api/fuzz` shape. |
| `dashboard/static/fixtures/mock-logs.json` | New | Fixture mirroring `/api/logs/:name` shape. |

---

## Task 1: Fixtures & mock mode wiring

Set up the offline dev path so every later task can be verified in a plain browser.

**Files:**
- Create: `dashboard/static/fixtures/mock.json`
- Create: `dashboard/static/fixtures/mock-fuzz.json`
- Create: `dashboard/static/fixtures/mock-logs.json`

- [ ] **Step 1: Create `dashboard/static/fixtures/mock.json`**

```json
{
  "nodes": [
    {
      "name": "rippled-0", "type": "rippled", "status": "ok", "server_state": "proposing",
      "validated_ledger": { "seq": 8421902, "hash": "A1B2C3D4E5F60718", "age": 2 },
      "ledger_current_index": 8421903, "peers": 4, "uptime": 3725, "build_version": "2.5.0",
      "last_close": { "proposers": 5, "converge_time_s": 3.2 }
    },
    {
      "name": "rippled-1", "type": "rippled", "status": "ok", "server_state": "proposing",
      "validated_ledger": { "seq": 8421902, "hash": "A1B2C3D4E5F60718", "age": 2 },
      "ledger_current_index": 8421903, "peers": 4, "uptime": 3725, "build_version": "2.5.0",
      "last_close": { "proposers": 5, "converge_time_s": 3.2 }
    },
    {
      "name": "rippled-2", "type": "rippled", "status": "ok", "server_state": "full",
      "validated_ledger": { "seq": 8421901, "hash": "BEEFCAFED00D1234", "age": 5 },
      "ledger_current_index": 8421902, "peers": 4, "uptime": 3725, "build_version": "2.5.0",
      "last_close": { "proposers": 5, "converge_time_s": 3.2 }
    },
    {
      "name": "goxrpl-0", "type": "goxrpl", "status": "ok", "server_state": "validating",
      "validated_ledger": { "seq": 8421902, "hash": "A1B2C3D4E5F60718", "age": 2 },
      "ledger_current_index": 8421903, "peers": 4, "uptime": 1820, "build_version": "0.9.0-dev",
      "last_close": { "proposers": 5, "converge_time_s": 3.6 }
    },
    {
      "name": "goxrpl-1", "type": "goxrpl", "status": "unreachable", "error": "connection refused"
    }
  ]
}
```

- [ ] **Step 2: Create `dashboard/static/fixtures/mock-fuzz.json`**

```json
{
  "txs_submitted_total": 12450,
  "txs_applied_total": 12421,
  "divergences_total": 7,
  "crashes_total": 1,
  "current_seed": "f3a1c8d2",
  "divergences_total_by_layer": {
    "consensus": 3,
    "ledger": 2,
    "tx-engine": 1,
    "amm": 1
  }
}
```

- [ ] **Step 3: Create `dashboard/static/fixtures/mock-logs.json`**

```json
{
  "state": {
    "status": "ok", "server_state": "proposing",
    "validated_ledger": { "seq": 8421902, "hash": "A1B2C3D4", "age": 2 },
    "peers": 4, "build_version": "2.5.0", "complete_ledgers": "8420000-8421902",
    "last_close": { "proposers": 5, "converge_time_s": 3.2 }
  },
  "logs": [
    { "ts": "2026-05-11T10:58:01.123Z", "level": "proposing", "message": "closed ledger 8421901" },
    { "ts": "2026-05-11T10:58:04.456Z", "level": "validating", "message": "accepted proposal" },
    { "ts": "2026-05-11T10:58:05.789Z", "level": "proposing", "message": "closed ledger 8421902 hash A1B2..." },
    { "ts": "2026-05-11T10:58:08.012Z", "level": "info", "message": "peers=4 latency=42ms" }
  ]
}
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/fixtures/
git commit -m "dashboard: add mock fixtures for offline UI dev"
```

---

## Task 2: HTML shell + tab routing

Replace `index.html` with the poster shell. Wire a minimal hash router that toggles `.active` on panels and tabs. No styling yet — pure structure.

**Files:**
- Modify (full rewrite): `dashboard/static/index.html`
- Modify (full rewrite): `dashboard/static/app.js`

- [ ] **Step 1: Write the new `dashboard/static/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>XRPL Confluence</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap">
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <main class="poster" data-tab="overview">

    <nav class="tabs" role="tablist">
      <button class="tab active" data-tab="overview" role="tab">Overview</button>
      <button class="tab" data-tab="nodes" role="tab">Nodes</button>
      <button class="tab" data-tab="topology" role="tab">Topology</button>
      <button class="tab" data-tab="timeline" role="tab">Timeline</button>
      <button class="tab" data-tab="fuzzer" role="tab">Fuzzer</button>
      <div class="sync" id="sync-badge">
        <span class="sync-dot"></span>
        <span class="sync-label" id="sync-label">CONNECTING</span>
      </div>
    </nav>

    <section class="panel active" data-panel="overview" role="tabpanel">
      <h1 class="title">Network Overview.</h1>
      <p class="subtitle">Live interop status across goXRPL ↔ rippled.</p>
      <div class="kpis kpis-2x2" id="overview-kpis"></div>
      <table class="summary" id="overview-summary"><tbody></tbody></table>
    </section>

    <section class="panel" data-panel="nodes" role="tabpanel">
      <h1 class="title">Nodes.</h1>
      <p class="subtitle">Click any card to inspect.</p>
      <div class="node-grid" id="node-grid"></div>
    </section>

    <section class="panel" data-panel="topology" role="tabpanel">
      <h1 class="title">Topology.</h1>
      <p class="subtitle">Click a node to inspect its logs.</p>
      <svg id="topology-svg" class="topology-svg" viewBox="0 0 700 520" preserveAspectRatio="xMidYMid meet"></svg>
    </section>

    <section class="panel" data-panel="timeline" role="tabpanel">
      <h1 class="title">Ledger Timeline.</h1>
      <p class="subtitle">Latest closes, newest at bottom.</p>
      <div class="timeline" id="timeline-list"></div>
      <button class="new-chip" id="timeline-new-chip" hidden>↓ <span id="timeline-new-count">0</span> new</button>
    </section>

    <section class="panel" data-panel="fuzzer" role="tabpanel">
      <h1 class="title">Fuzzer.</h1>
      <p class="subtitle">Seed <span id="fuzz-seed-inline" class="mono">—</span></p>
      <div class="kpis kpis-1x5" id="fuzz-kpis"></div>
      <table class="layers" id="fuzz-by-layer">
        <thead><tr><th>Layer</th><th>Divergences</th></tr></thead>
        <tbody></tbody>
      </table>
    </section>

    <aside class="drawer" id="drawer" hidden>
      <div class="drawer-resize" id="drawer-resize"></div>
      <header class="drawer-head">
        <span class="drawer-title" id="drawer-title">—</span>
        <button class="drawer-close" id="drawer-close" aria-label="Close">✕</button>
      </header>
      <div class="drawer-state" id="drawer-state"></div>
      <div class="drawer-logs" id="drawer-logs"></div>
    </aside>

    <footer class="footer">
      <span class="footer-line">Last update · <span id="footer-age">—</span> <span id="footer-ctx"></span></span>
    </footer>

  </main>
  <script src="/app.js"></script>
</body>
</html>
```

- [ ] **Step 2: Write the new `dashboard/static/app.js` skeleton**

```javascript
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

  // ── Init ────────────────────────────────────────────────────
  document.addEventListener("DOMContentLoaded", () => {
    for (const el of document.querySelectorAll(".tab")) {
      el.addEventListener("click", () => setTab(el.dataset.tab));
    }
    window.addEventListener("hashchange", () => setTab(currentTab()));
    setTab(currentTab());
  });
})();
```

- [ ] **Step 3: Verify in browser**

```bash
cd dashboard/static && python3 -m http.server 8765
```

Open `http://localhost:8765/?mock=1`. Click each tab button and confirm the URL hash changes (`#overview`, `#nodes`, etc.) and that only the active panel is visible (will look unstyled until Task 3). Reload with `#topology` in the URL and confirm Topology is active on first paint.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/index.html dashboard/static/app.js
git commit -m "dashboard: scaffold poster shell with hash-routed tabs"
```

---

## Task 3: Design tokens + poster frame + tab bar + sync badge

Replace `style.css` with the foundational layer: tokens, body canvas, the white poster card, the tab bar, and the sync badge. Per-panel styling lands in later tasks.

**Files:**
- Modify (full rewrite): `dashboard/static/style.css`

- [ ] **Step 1: Write the new `dashboard/static/style.css`**

```css
:root {
  --ink: #0a0a0a;
  --paper: #fcfbf7;
  --canvas: #0a0a0a;
  --mid: #888;
  --rule: #d0d0d0;
  --cobalt: #1f3dff;
  --mustard: #d4a800;
  --red: #c8201a;

  --serif: "GT Sectra", "Tiempos Headline", Georgia, "Times New Roman", serif;
  --sans:  "Inter", system-ui, -apple-system, "Segoe UI", sans-serif;
  --mono:  "JetBrains Mono", "SF Mono", "Cascadia Code", "Fira Code", monospace;

  --w-narrow: 720px;
  --w-wide:   960px;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

html, body {
  background: var(--canvas);
  font-family: var(--sans);
  color: var(--ink);
  min-height: 100vh;
}

body {
  display: flex;
  justify-content: center;
  padding: 96px 24px;
}

/* Poster */
.poster {
  background: var(--paper);
  border: 1px solid var(--ink);
  width: 100%;
  max-width: var(--w-narrow);
  position: relative;
  display: flex;
  flex-direction: column;
  transition: max-width 0.18s ease;
}
.poster[data-tab="nodes"],
.poster[data-tab="topology"] { max-width: var(--w-wide); }

/* Tabs */
.tabs {
  display: flex;
  align-items: stretch;
  border-bottom: 1px solid var(--ink);
}
.tab {
  background: transparent;
  border: none;
  border-right: 1px solid var(--ink);
  padding: 10px 14px 11px;
  font: 500 11px/1 var(--sans);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink);
  cursor: pointer;
}
.tab:hover { background: rgba(0,0,0,0.04); }
.tab.active { background: var(--ink); color: var(--paper); }

/* Sync badge — lives in the tab bar, right-justified */
.sync {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 14px;
  font: 500 11px/1 var(--sans);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.sync-dot {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--mid);
  transition: background-color 0.4s ease;
}
.sync.connected   .sync-dot { background: var(--cobalt); }
.sync.reconnecting .sync-dot { background: var(--mustard); }
.sync.offline     .sync-dot { background: var(--red); }

/* Panels */
.panel { display: none; padding: 32px 36px; }
.panel.active { display: block; }

/* Type — placeholders, refined per panel in later tasks */
.title    { font: 500 30px/1.1 var(--serif); letter-spacing: -0.01em; margin-bottom: 6px; }
.subtitle { font: italic 13px/1.4 var(--serif); color: var(--mid); margin-bottom: 28px; }
.mono     { font-family: var(--mono); }

/* Footer */
.footer {
  border-top: 1px solid var(--ink);
  padding: 10px 36px 14px;
  font: italic 11px/1.3 var(--serif);
  color: var(--mid);
}
.footer .mono { font-style: normal; }
```

- [ ] **Step 2: Verify in browser**

Same dev server as Task 2. Reload `http://localhost:8765/?mock=1`. Expected: deep black canvas, a single paper-white card centered with 96px gutter, five tab buttons across the top with a vertical rule between each, `CONNECTING` badge on the right of the tab bar (with a gray dot — colors land when Task 5 wires status). Clicking Nodes or Topology widens the card to 960px; switching back narrows it. Active tab is black-on-paper. Footer reads `Last update · — ` for now.

- [ ] **Step 3: Commit**

```bash
git add dashboard/static/style.css
git commit -m "dashboard: foundational tokens, poster frame, tabs, sync badge"
```

---

## Task 4: Data layer — preserve SSE/poll, route through fixture loader

Port the existing data fetch logic into the new `app.js`, plus an offline path that loads fixtures when `?mock=1`.

**Files:**
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append the data layer below the routing code in `app.js`**

Add this block after the Tab routing section, before the `DOMContentLoaded` listener:

```javascript
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
```

- [ ] **Step 2: Add footer tick + boot into the `DOMContentLoaded` listener**

Replace the existing `DOMContentLoaded` block with:

```javascript
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
```

- [ ] **Step 3: Verify in browser**

Reload `http://localhost:8765/?mock=1`. Expected: sync badge flips to `CONNECTED` (still gray dot — token wired in Task 3 needs the right class; verify by inspecting that `.sync.connected` is on the badge element). Footer ticks `Last update · 1s ago`, `2s ago`, etc. Then reload without `?mock=1` and confirm the badge becomes `RECONNECTING` (no backend, but fetch fails gracefully without a hang).

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: data layer with mock fixture path + sync state + footer tick"
```

---

## Task 5: Overview tab

KPI grid (Nodes online · Latest ledger · Proposers · Converge time) and the divergence summary table.

**Files:**
- Modify: `dashboard/static/style.css` (Overview rules)
- Modify: `dashboard/static/app.js` (Overview renderer)

- [ ] **Step 1: Append Overview rules to `style.css`**

```css
/* Overview KPIs */
.kpis {
  display: grid;
  gap: 28px 32px;
  margin-bottom: 36px;
}
.kpis-2x2 { grid-template-columns: 1fr 1fr; }
.kpis-1x5 { grid-template-columns: repeat(5, 1fr); }

.kpi { padding-top: 12px; border-top: 1px solid var(--ink); }
.kpi-lbl  { font: 500 11px/1 var(--sans); letter-spacing: 0.14em; text-transform: uppercase; }
.kpi-num  { font: 500 32px/1.05 var(--serif); letter-spacing: -0.01em; margin-top: 12px; }
.kpi-num.mono { font: 500 22px/1.1 var(--mono); letter-spacing: 0; }

/* Summary table */
.summary { width: 100%; border-collapse: collapse; }
.summary tr { border-top: 1px solid var(--ink); }
.summary tr:last-child { border-bottom: 1px solid var(--ink); }
.summary td {
  padding: 10px 0;
  font: 13px/1.4 var(--sans);
  vertical-align: baseline;
}
.summary td:first-child  { font-weight: 500; }
.summary td:nth-child(2) { color: var(--mid); }
.summary td:last-child   { text-align: right; font: 500 16px/1 var(--serif); }
```

- [ ] **Step 2: Append Overview renderer to `app.js`**

Add inside the IIFE, before `DOMContentLoaded`:

```javascript
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
```

- [ ] **Step 3: Verify in browser**

Reload `http://localhost:8765/?mock=1`. Expected on the Overview tab: 2×2 KPI grid with thin black top rules, uppercase sans labels, large serif numbers (`4 / 5`, `8,421,902`, `5`, `3.4s`). Below it a 4-row summary table with `Ledger spread · synced · 1`, `Unreachable nodes · needs attention · 1`, `Fuzzer divergences · investigating · 7`, `Fuzzer crashes · open · 1`. No colors yet beyond ink/mid-gray.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: overview tab — KPI grid + divergence summary"
```

---

## Task 6: Nodes tab

3-column card grid; each card has a status-colored top rule, serif name, mono peer ID, mini KPI line.

**Files:**
- Modify: `dashboard/static/style.css`
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append Nodes rules to `style.css`**

```css
/* Nodes grid */
.node-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
}
.node-card {
  border: 1px solid var(--ink);
  border-top-width: 2px;
  padding: 16px 16px 14px;
  background: var(--paper);
  cursor: pointer;
  transition: background 0.18s ease;
}
.node-card:hover { background: rgba(0,0,0,0.03); }
.node-card.health-ok    { border-top-color: var(--cobalt); }
.node-card.health-warn  { border-top-color: var(--mustard); }
.node-card.health-err   { border-top-color: var(--red); }
.node-card.selected     { outline: 2px solid var(--ink); outline-offset: -2px; }

.node-name { font: 500 18px/1.1 var(--serif); margin-bottom: 4px; }
.node-id   { font: 11px/1.3 var(--mono); color: var(--mid); margin-bottom: 14px;
             overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.node-mini { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
.node-mini .l { font: 500 10px/1 var(--sans); letter-spacing: 0.12em; text-transform: uppercase; color: var(--mid); }
.node-mini .v { font: 500 13px/1.2 var(--mono); margin-top: 4px; color: var(--ink); }
```

- [ ] **Step 2: Append Nodes renderer to `app.js`**

```javascript
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
```

- [ ] **Step 3: Verify in browser**

Switch to `#nodes`. Expect a 3-column grid of 5 cards with thin colored top rules: `rippled-0/1/2` and `goxrpl-0` get cobalt rules, `goxrpl-1` gets a red rule. Each card shows serif name, gray mono version/error string, then `LEDGER · PEERS · LAG` mini row in mono numbers. Hover lightens the background. Clicking logs `openDrawer rippled-0` etc. to the console.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: nodes tab — card grid with health rule"
```

---

## Task 7: Topology tab

Full-width SVG, nodes laid out on a circle, edges 1px solid (cobalt for `ok ↔ ok` pairs). Click handler opens drawer (placeholder until Task 8).

**Files:**
- Modify: `dashboard/static/style.css`
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append Topology rules to `style.css`**

```css
.topology-svg { width: 100%; height: auto; display: block; margin: 4px 0 12px; }
.topo-link        { stroke: var(--ink); stroke-width: 1; }
.topo-link.active { stroke: var(--cobalt); }
.topo-node-circle           { fill: var(--ink); stroke: var(--paper); stroke-width: 2; transition: fill 0.4s ease; }
.topo-node-circle.unreachable { fill: var(--red); }
.topo-node-circle.warn        { fill: var(--mustard); }
.topo-node-circle.hovered, .topo-node-circle.selected {
  stroke: var(--ink); stroke-width: 3;
}
.topo-node-label {
  font: italic 11px/1 var(--serif);
  fill: var(--ink);
  text-anchor: middle;
  dominant-baseline: hanging;
  pointer-events: none;
}
.topo-node      { cursor: pointer; }
```

- [ ] **Step 2: Append Topology renderer to `app.js`**

```javascript
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
```

- [ ] **Step 3: Verify in browser**

Switch to `#topology`. Expect a centered SVG with 5 small ink-filled circles arranged on a circle, thin black lines between every pair, cobalt lines between `ok ↔ ok` pairs. Each circle has its node name in italic serif below it. The `goxrpl-1` circle is red. Clicking a circle logs `openDrawer ...`.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: topology tab — full-width trust graph"
```

---

## Task 8: Drawer (shared by Nodes + Topology)

Slide-up from the bottom of the poster, holds the per-node log stream. Drag-resize handle on top edge.

**Files:**
- Modify: `dashboard/static/style.css`
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append drawer rules to `style.css`**

```css
.drawer {
  position: relative;
  border-top: 1px solid var(--ink);
  background: var(--paper);
  display: flex;
  flex-direction: column;
  max-height: 60%;
  height: 30%;
  min-height: 120px;
  overflow: hidden;
  transform: translateY(0);
  transition: height 0.18s ease;
}
.drawer[hidden] { display: none; }
.drawer-resize {
  position: absolute; top: -2px; left: 0; right: 0; height: 4px;
  cursor: ns-resize;
  background: transparent;
}
.drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 36px 9px;
  border-bottom: 1px solid var(--rule);
}
.drawer-title {
  font: 500 11px/1 var(--sans);
  letter-spacing: 0.14em;
  text-transform: uppercase;
}
.drawer-close {
  background: none; border: none; font: 14px/1 var(--sans); cursor: pointer;
  width: 22px; height: 22px; color: var(--ink);
}
.drawer-state {
  padding: 8px 36px;
  font: 12px/1.4 var(--sans);
  color: var(--mid);
  border-bottom: 1px solid var(--rule);
}
.drawer-state b { font-weight: 500; color: var(--ink); }
.drawer-logs {
  flex: 1; overflow-y: auto; overflow-x: auto;
  padding: 8px 36px 14px;
  font: 11px/1.55 var(--mono);
  white-space: pre;
}
.drawer-logs .row { display: flex; gap: 14px; }
.drawer-logs .ts  { color: var(--mid); }
.drawer-logs .lvl-ok  { color: var(--cobalt); }
.drawer-logs .lvl-err { color: var(--red); }
```

- [ ] **Step 2: Replace the placeholder `openDrawer` in `app.js` with the full implementation**

```javascript
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
```

- [ ] **Step 3: Wire close button + resize in `DOMContentLoaded`**

Add inside the `DOMContentLoaded` callback before `connectSSE()`:

```javascript
    document.getElementById("drawer-close").addEventListener("click", closeDrawer);
    wireDrawerResize();
```

- [ ] **Step 4: Verify in browser**

On `#nodes` click `rippled-0`. Drawer slides up from the bottom of the poster, shows header `RIPPLED-0 · LOGS`, then a one-line state summary, then the mock log lines with mono timestamps. Cobalt color on `proposing` / `validating` levels. Click ✕ to close. Switch to `#topology`, click any circle — same drawer opens, that circle gains a thicker ring. Drag the top edge of the drawer to resize.

- [ ] **Step 5: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: shared drawer for nodes + topology log inspection"
```

---

## Task 9: Timeline tab

Newest-at-bottom vertical list. Each row: serif ledger number, vertical rule, mono close-time + hash, then 1 dot per node (cobalt = matched, red = diverged). Pin-on-scroll with `↓ N new` chip.

**Files:**
- Modify: `dashboard/static/style.css`
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append Timeline rules to `style.css`**

```css
.timeline {
  border-top: 1px solid var(--ink);
  max-height: 380px;
  overflow-y: auto;
  position: relative;
}
.timeline-row {
  display: grid;
  grid-template-columns: 120px 1px 1fr;
  align-items: baseline;
  border-bottom: 1px solid var(--rule);
  padding: 10px 0;
}
.timeline-seq  { font: 500 18px/1 var(--serif); }
.timeline-rule { background: var(--ink); align-self: stretch; }
.timeline-meta { display: flex; gap: 14px; align-items: center; padding-left: 14px; font: 12px/1 var(--mono); color: var(--mid); }
.timeline-hash { color: var(--ink); }
.timeline-dots { margin-left: auto; display: flex; gap: 5px; }
.timeline-dots span { width: 6px; height: 6px; border-radius: 50%; background: var(--cobalt); transition: background 0.4s ease; }
.timeline-dots span.diverged { background: var(--red); }

.new-chip {
  position: sticky; bottom: 8px; align-self: center; margin: 8px auto 0;
  font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase;
  padding: 6px 10px; background: var(--ink); color: var(--paper); border: none; cursor: pointer;
  display: block;
}
.new-chip[hidden] { display: none; }
.timeline-empty {
  padding: 36px 0; text-align: center;
  font: italic 12px/1.4 var(--serif); color: var(--mid);
}
```

- [ ] **Step 2: Append Timeline renderer + accumulator to `app.js`**

```javascript
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
        const reference = [...row.byNode.values()][0];
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
```

- [ ] **Step 3: Wire scroll handler in `DOMContentLoaded`**

Add inside the `DOMContentLoaded` block, after `wireDrawerResize();`:

```javascript
    wireTimelineScroll();
```

- [ ] **Step 4: Verify in browser**

Switch to `#timeline`. Expect a single row at first (fixture has 1 validated seq). Reload — since mock returns the same seq, you'll see the empty state shift once. To stress the pin-chip: scroll up inside the list (resize browser to force scroll), then in DevTools console run `closeRows.set(8421999, { time: new Date(), hash: "deadbeefcafe", byNode: new Map([["rippled-0","match"]]) }); renderTimeline(latest);` — chip should appear `↓ 1 new` at the bottom; clicking it scrolls down and hides itself.

- [ ] **Step 5: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: timeline tab — per-close rows with quorum dots"
```

---

## Task 10: Fuzzer tab

5-up KPI row, divergences-by-layer table with red left border on crashed layers.

**Files:**
- Modify: `dashboard/static/style.css`
- Modify: `dashboard/static/app.js`

- [ ] **Step 1: Append Fuzzer rules to `style.css`**

```css
.layers { width: 100%; border-collapse: collapse; margin-top: 28px; }
.layers thead th {
  text-align: left;
  font: 500 11px/1 var(--sans);
  letter-spacing: 0.14em;
  text-transform: uppercase;
  padding: 8px 0;
  border-bottom: 1px solid var(--ink);
}
.layers tbody td { padding: 10px 0; font: 14px/1.3 var(--sans); border-bottom: 1px solid var(--rule); }
.layers tbody td:last-child { text-align: right; font: 500 18px/1 var(--serif); }
.layers tbody tr.crashed td:first-child { border-left: 2px solid var(--red); padding-left: 10px; }
```

- [ ] **Step 2: Append Fuzzer renderer to `app.js`**

```javascript
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
```

- [ ] **Step 3: Call `updateFooterContext()` from `notify` and on tab change**

Edit the existing `notify` function to call `updateFooterContext()`, and add the same call to `setTab` after toggling classes:

```javascript
  function notify() {
    lastUpdate = Date.now();
    for (const fn of renderers) {
      try { fn(latest, latestFuzz); } catch (e) { console.error(e); }
    }
    updateFooterContext();
  }
```

And in `setTab`, just before the closing brace:

```javascript
    updateFooterContext();
```

- [ ] **Step 4: Verify in browser**

Switch to `#fuzzer`. Expect: subtitle reads `Seed #f3a1c8d2`. 5 KPIs in a single row (`SUBMITTED · APPLIED · DIVERGENCES · CRASHES · SEED`), each with a top rule + uppercase label + serif/mono number. The bottom table lists the 4 layers (`amm · consensus · ledger · tx-engine` alphabetised). Because the mock has `crashes_total = 1`, every row gets a red left border (whole-table crashed state). Footer reads `Last update · 2s ago · seed #f3a1c8d2`. Switch to `#overview` — footer flips to `· ledger 8,421,902`.

- [ ] **Step 5: Commit**

```bash
git add dashboard/static/style.css dashboard/static/app.js
git commit -m "dashboard: fuzzer tab — KPI row + layer table"
```

---

## Task 11: Responsive + print

Final polish: 1040px and 640px breakpoints, print stylesheet.

**Files:**
- Modify: `dashboard/static/style.css`

- [ ] **Step 1: Append media queries to `style.css`**

```css
@media (max-width: 1040px) {
  .poster[data-tab="nodes"],
  .poster[data-tab="topology"] { max-width: 100%; }
  body { padding: 32px 16px; }
  .tabs { overflow-x: auto; -webkit-overflow-scrolling: touch; }
  .tab { white-space: nowrap; }
}

@media (max-width: 640px) {
  .panel { padding: 24px 20px; }
  .kpis-2x2 { grid-template-columns: 1fr; gap: 18px; }
  .kpis-1x5 { grid-template-columns: 1fr 1fr; gap: 18px; }
  .node-grid { grid-template-columns: 1fr; }
  .footer { padding-left: 20px; padding-right: 20px; }
  .drawer { height: 50%; }
  .drawer-head, .drawer-state, .drawer-logs { padding-left: 20px; padding-right: 20px; }
}

@media print {
  body { background: var(--paper); padding: 0; }
  .poster { border: none; max-width: 100%; }
  .sync, .footer, .drawer, .new-chip { display: none !important; }
  .tabs .tab { border: none; padding: 4px 8px; }
  .tabs .tab:not(.active) { display: none; }
}
```

- [ ] **Step 2: Verify in browser**

Resize the browser to 800px width — Topology and Nodes posters now span full width, tabs become a horizontal scroller. Resize to 600px — Overview KPIs collapse to a single column, Nodes grid is 1 column, Fuzzer KPIs are 2 columns. Then File → Print Preview: only the active tab's content shows on a clean white page with no sync badge, no footer, no drawer.

- [ ] **Step 3: Commit**

```bash
git add dashboard/static/style.css
git commit -m "dashboard: responsive breakpoints + print stylesheet"
```

---

## Task 12: Live data smoke test + cleanup

Verify everything end-to-end against a real Kurtosis enclave, then strip any dev-only leftovers.

- [ ] **Step 1: Start a live enclave**

```bash
make soak
```

Wait for the dashboard to be reachable (the Makefile prints the mapped port; e.g. `http://localhost:50123`).

- [ ] **Step 2: Open without `?mock=1` and walk every tab**

In the browser:
- Overview KPIs populate with live data, footer ticks.
- Nodes shows the real node count (will be 7 in soak).
- Topology renders all nodes; clicking each opens the drawer with live `/api/logs/:name` data; the drawer polls every 2s and rows append.
- Timeline rows accumulate as new closes arrive; scroll up and confirm the `↓ N new` chip appears as new rows arrive.
- Fuzzer panel stays empty (no fuzz sidecar in soak) — confirm no console errors.

- [ ] **Step 3: Spot-check error states**

```bash
make soak-down
```

Watch the sync badge flip `CONNECTED → RECONNECTING → OFFLINE`. No console errors. No spinners or loading states.

- [ ] **Step 4: Confirm spec deletions**

Run `grep -n "loading\|spinner\|fadeIn\|@keyframes\|hint\|border-radius" dashboard/static/style.css` and confirm:
- No `.loading` or `.loading-spinner` rules remain.
- No `fadeIn` keyframes remain.
- No `border-radius` declarations remain (poster, cards, drawer, KPIs are all sharp corners).
- No `.hint` class remains.

If any remain, delete them and commit.

- [ ] **Step 5: Final commit**

```bash
git add -u
git commit -m "dashboard: live data smoke test + cleanup of stale rules"
```

---

## Self-Review

**Spec coverage** — each section/requirement of `docs/design/2026-05-11-dashboard-ui-rework-design.md` maps to a task:

| Spec section | Task |
|---|---|
| §1 Shell (canvas, paper, max-widths, tab bar, sync badge, footer) | 2, 3 |
| §2 Typography | 3 (tokens), refined in 5/6/7/8/9/10 |
| §3 Color system + accents | 3, 6 (health rules), 7 (topology), 9 (dots), 10 (red border), 11 (transitions on .topo-node-circle and .sync-dot) |
| §4.1 Overview | 5 |
| §4.2 Nodes | 6 + drawer (8) |
| §4.3 Topology | 7 + drawer (8) |
| §4.4 Timeline | 9 |
| §4.5 Fuzzer | 10 |
| §5 Real-time behavior (silent refresh, footer tick, ↓ N new chip, sync states) | 4, 9, 12 |
| §6 Motion (180ms drawer slide, 400ms color crossfade) | 7 (`transition: stroke 0.4s`), 8 (`transition: height 0.18s`), 11 (responsive transitions) |
| §7 Responsive (1040, 640, print) | 11 |
| §8 Deletions | 12 step 4 |

**Notes / known gaps that ship as-is:** the spec's "180ms drawer slide from the bottom edge" is implemented as a height transition rather than a transform-based slide. Visually equivalent and avoids the complexity of an offscreen→onscreen animation that fights with the resizable height. If pure-slide is desired, swap `height` → `transform: translateY` in Task 8.
