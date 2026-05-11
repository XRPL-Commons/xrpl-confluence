# Dashboard Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert the XRPL Confluence dashboard from a tabbed poster to a three-column workbench (nodes rail / main view / inspector) with global selection, URL-routed state, keyboard nav, filters, pause/play, and a command palette. Style is preserved; interaction model and layout change.

**Architecture:** Vanilla HTML/CSS/JS in `dashboard/static/`. A single store object (subscribe/notify) replaces ad-hoc module vars and the `renderers[]` array. URL `location.hash` carries `view`, `selected node`, and key filters. The existing renderers (`renderTimeline`, `renderTopology`, `renderFuzzer`) are reused; their containers and selection styling change. No new dependencies, no build step. Server (`dashboard/server.js`) is untouched.

**Tech Stack:** HTML, CSS (custom properties, grid), ES2020 JS (IIFE module, no transpile), SSE + fetch, served by `dashboard/server.js` (Node 22).

**Spec:** `xrpl-confluence/docs/design/2026-05-11-dashboard-workbench-design.md`

---

## File Structure

All changes confined to three files under `dashboard/static/`. No new files.

- `dashboard/static/index.html` — restructured shell. Drop `.tabs`, `.drawer`, `.footer`, `#overview-summary`. Add `#topbar`, `#rail-nodes`, `#main-switch`, `#main-view` (with panel containers for timeline/topology/fuzzer), `#inspector`, `#palette`, `#cheatsheet`.
- `dashboard/static/style.css` — replace `.poster` flex with grid `220px 1fr 320px`. Keep `:root`, type ramp, palette, KPI/summary/timeline/fuzzer block styles. Add styles for top bar, segmented control, inspector chips, palette/cheatsheet overlays. Drop `.tab`, `.tabs`, `.drawer*`, `.footer`, `@media print`.
- `dashboard/static/app.js` — refactor:
  - `createStore` (subscribe/notify).
  - `parseHash` / `pushHash` (URL ↔ state).
  - `setView`, `setSelected`, `setPaused`, `setFilter` mutators.
  - Per-panel subscribe (`subscribeTopbar`, `subscribeRail`, `subscribeMain`, `subscribeInspector`).
  - Reuse `renderTimeline`, `renderTopology`, `renderFuzzer`, `renderDrawer` bodies (drawer becomes inspector).
  - Replace `openDrawer/closeDrawer` with selection-driven inspector population.
  - Keyboard handler (delegated `keydown` on `document`).
  - Command palette wiring.

The existing single-file structure stays; the file grows to ~700 lines, which is still manageable. No premature splitting.

---

## Task 1: Skeleton — three-column shell with empty regions

**Files:**
- Modify: `dashboard/static/index.html`
- Modify: `dashboard/static/style.css`

Replace the tabbed body with the three-column grid shell. No JS changes yet — every region renders static placeholder content so the layout can be inspected with `?mock=1` and the existing `app.js` continues to run (we'll wire it in later tasks). The Overview tab content stays accessible via the existing `#overview-kpis` and `#overview-summary` for now (parked inside the inspector's empty-state container).

- [ ] **Step 1: Replace `index.html` body**

Open `dashboard/static/index.html` and replace the `<body>` contents:

```html
<body>
  <main class="workbench">

    <header class="topbar">
      <span class="brand">XRPL Confluence</span>
      <div class="sync" id="sync-badge">
        <span class="sync-dot"></span>
        <span class="sync-label" id="sync-label">CONNECTING</span>
      </div>
      <button class="pause-btn" id="pause-btn" aria-pressed="false" title="Pause (space)">⏸</button>
      <span class="pending-chip" id="pending-chip" hidden>+<span id="pending-count">0</span> pending</span>
      <button class="palette-btn" id="palette-btn" title="Command palette (⌘K)">⌘K</button>
      <span class="topbar-ctx" id="topbar-ctx">—</span>
    </header>

    <aside class="rail rail-nodes" data-pane="rail" id="rail-nodes">
      <div class="rail-head">
        <span class="rail-label">Nodes</span>
        <input class="rail-filter" id="rail-filter" type="text" placeholder="/ filter" />
      </div>
      <div class="node-list" id="node-list"></div>
      <div class="rail-kpis" id="rail-kpis"></div>
    </aside>

    <section class="main" data-pane="main" id="main">
      <nav class="main-switch" id="main-switch" role="tablist">
        <button class="seg active" data-view="timeline" role="tab">Timeline<span class="seg-badge" id="seg-timeline-badge" hidden></span></button>
        <button class="seg" data-view="topology" role="tab">Topology</button>
        <button class="seg" data-view="fuzzer" role="tab">Fuzzer<span class="seg-badge" id="seg-fuzzer-badge" hidden></span></button>
      </nav>

      <div class="view view-timeline active" data-view="timeline">
        <h1 class="title">Ledger Timeline.</h1>
        <div class="view-controls">
          <input class="view-filter" id="timeline-filter" type="text" placeholder="/ filter seq or hash" />
          <label class="chip-toggle"><input type="checkbox" id="timeline-only-diverged" /> only divergences</label>
        </div>
        <div class="timeline" id="timeline-list"></div>
        <button class="new-chip" id="timeline-new-chip" hidden>↓ <span id="timeline-new-count">0</span> new</button>
      </div>

      <div class="view view-topology" data-view="topology">
        <h1 class="title">Topology.</h1>
        <p class="subtitle">Click a node to inspect its logs.</p>
        <svg id="topology-svg" class="topology-svg" viewBox="0 0 700 520" preserveAspectRatio="xMidYMid meet"></svg>
      </div>

      <div class="view view-fuzzer" data-view="fuzzer">
        <h1 class="title">Fuzzer.</h1>
        <p class="subtitle">Seed <span id="fuzz-seed-inline" class="mono">—</span></p>
        <div class="kpis kpis-1x5" id="fuzz-kpis"></div>
        <table class="layers" id="fuzz-by-layer">
          <thead><tr><th>Layer</th><th>Divergences</th></tr></thead>
          <tbody></tbody>
        </table>
      </div>
    </section>

    <aside class="rail rail-inspector" data-pane="inspector" id="inspector">
      <header class="inspector-head">
        <span class="inspector-title" id="inspector-title">Fleet</span>
        <button class="inspector-clear" id="inspector-clear" aria-label="Clear selection" hidden>✕</button>
      </header>
      <div class="inspector-state" id="inspector-state"></div>
      <div class="inspector-empty" id="inspector-empty">
        <table class="summary" id="overview-summary"><tbody></tbody></table>
      </div>
      <div class="inspector-logs-head" id="inspector-logs-head" hidden>
        <div class="log-levels">
          <button class="log-chip active" data-level="ok">ok</button>
          <button class="log-chip active" data-level="warn">warn</button>
          <button class="log-chip active" data-level="err">err</button>
        </div>
        <input class="log-filter" id="log-filter" type="text" placeholder="/ filter" />
        <label class="chip-toggle"><input type="checkbox" id="log-follow" checked /> follow</label>
      </div>
      <div class="inspector-logs" id="inspector-logs" hidden></div>
    </aside>

    <div class="overlay" id="palette" hidden>
      <div class="palette-card">
        <input class="palette-input" id="palette-input" placeholder="Type to filter…" />
        <ul class="palette-list" id="palette-list"></ul>
      </div>
    </div>

    <div class="overlay" id="cheatsheet" hidden>
      <div class="cheatsheet-card">
        <h2>Shortcuts</h2>
        <dl>
          <dt>1 / 2 / 3</dt><dd>Timeline / Topology / Fuzzer</dd>
          <dt>j / k</dt><dd>Walk focused list</dd>
          <dt>/</dt><dd>Focus filter</dd>
          <dt>esc</dt><dd>Clear filter → selection → collapse inspector</dd>
          <dt>space</dt><dd>Pause / resume</dd>
          <dt>n</dt><dd>Next divergence (Timeline)</dd>
          <dt>c</dt><dd>Copy focused log / state / row</dd>
          <dt>⌘K</dt><dd>Command palette</dd>
          <dt>tab</dt><dd>Cycle panes</dd>
          <dt>?</dt><dd>This cheatsheet</dd>
        </dl>
      </div>
    </div>

  </main>
  <script src="/app.js"></script>
</body>
```

- [ ] **Step 2: Replace `style.css` with workbench grid styles**

Open `dashboard/static/style.css` and replace its entire contents:

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

  --rail-w: 220px;
  --inspector-w: 320px;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

html, body { background: var(--canvas); font-family: var(--sans); color: var(--ink); min-height: 100vh; }
body { padding: 24px; }

.workbench {
  background: var(--paper);
  border: 1px solid var(--ink);
  min-height: calc(100vh - 48px);
  display: grid;
  grid-template-columns: var(--rail-w) 1fr var(--inspector-w);
  grid-template-rows: auto 1fr;
  grid-template-areas:
    "topbar topbar topbar"
    "rail   main   inspector";
}

/* Top bar */
.topbar {
  grid-area: topbar;
  display: flex; align-items: center; gap: 14px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--ink);
}
.brand { font: 500 14px/1 var(--serif); letter-spacing: -0.01em; }
.topbar .sync { margin-left: auto; display: flex; align-items: center; gap: 8px; font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase; }
.sync-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--mid); transition: background-color 0.4s ease; }
.sync.connected .sync-dot { background: var(--cobalt); }
.sync.reconnecting .sync-dot { background: var(--mustard); }
.sync.offline .sync-dot { background: var(--red); }
.pause-btn { background: none; border: 1px solid var(--ink); padding: 4px 8px; cursor: pointer; font: 500 11px/1 var(--sans); color: var(--ink); }
.pause-btn[aria-pressed="true"] { background: var(--ink); color: var(--paper); }
.pending-chip { font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase; color: var(--mustard); }
.pending-chip[hidden] { display: none; }
.palette-btn { background: none; border: 1px solid var(--ink); padding: 4px 8px; font: 500 11px/1 var(--mono); cursor: pointer; }
.topbar-ctx { font: italic 12px/1 var(--serif); color: var(--mid); }

/* Rail (nodes) */
.rail-nodes { grid-area: rail; border-right: 1px solid var(--ink); display: flex; flex-direction: column; }
.rail-head { padding: 14px 14px 10px; }
.rail-label { font: 500 11px/1 var(--sans); letter-spacing: 0.14em; text-transform: uppercase; display: block; margin-bottom: 8px; }
.rail-filter { width: 100%; border: none; border-bottom: 1px solid var(--rule); padding: 4px 0; font: 13px/1 var(--sans); background: transparent; outline: none; }
.rail-filter:focus { border-bottom-color: var(--ink); }
.node-list { flex: 1; overflow-y: auto; padding: 4px 0; }
.node-row { display: flex; align-items: center; gap: 10px; padding: 7px 14px; cursor: pointer; font: 13px/1 var(--sans); border-left: 2px solid transparent; }
.node-row:hover { background: rgba(0,0,0,0.03); }
.node-row.selected { background: var(--ink); color: var(--paper); }
.node-row.selected .node-pip { box-shadow: 0 0 0 1px var(--paper); }
.node-row.kb-focus { border-left-color: var(--cobalt); }
.node-pip { width: 8px; height: 8px; border-radius: 50%; background: var(--mid); flex-shrink: 0; }
.node-pip.health-ok { background: var(--cobalt); }
.node-pip.health-warn { background: var(--mustard); }
.node-pip.health-err { background: var(--red); }
.rail-kpis { border-top: 1px solid var(--rule); padding: 10px 14px 14px; }
.rail-kpis .kpi { padding-top: 8px; border-top: 1px solid var(--rule); margin-top: 8px; }
.rail-kpis .kpi:first-child { border-top: none; margin-top: 0; padding-top: 0; }
.rail-kpis .kpi-lbl { font: 500 10px/1 var(--sans); letter-spacing: 0.12em; text-transform: uppercase; color: var(--mid); }
.rail-kpis .kpi-num { font: 500 20px/1.05 var(--serif); letter-spacing: -0.01em; margin-top: 4px; }
.rail-kpis .kpi-num.mono { font: 500 16px/1.1 var(--mono); letter-spacing: 0; }

/* Main */
.main { grid-area: main; display: flex; flex-direction: column; min-width: 0; }
.main-switch { display: flex; gap: 0; padding: 12px 24px 0; border-bottom: 1px solid var(--ink); }
.seg { background: transparent; border: 1px solid var(--ink); border-bottom: none; padding: 8px 14px; font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase; cursor: pointer; color: var(--ink); margin-right: -1px; position: relative; top: 1px; }
.seg.active { background: var(--paper); border-bottom: 1px solid var(--paper); }
.seg:not(.active) { background: rgba(0,0,0,0.03); }
.seg-badge { display: inline-block; min-width: 16px; margin-left: 6px; padding: 1px 5px; border-radius: 8px; background: var(--red); color: var(--paper); font: 500 10px/1.2 var(--mono); }
.seg-badge[hidden] { display: none; }

.view { display: none; padding: 24px 28px; overflow-y: auto; flex: 1; }
.view.active { display: block; }
.title { font: 500 26px/1.1 var(--serif); letter-spacing: -0.01em; margin-bottom: 6px; }
.subtitle { font: italic 13px/1.4 var(--serif); color: var(--mid); margin-bottom: 20px; }
.mono { font-family: var(--mono); }

.view-controls { display: flex; gap: 14px; align-items: center; margin-bottom: 12px; }
.view-filter { border: none; border-bottom: 1px solid var(--rule); padding: 4px 0; font: 13px/1 var(--sans); background: transparent; outline: none; min-width: 220px; }
.view-filter:focus { border-bottom-color: var(--ink); }
.chip-toggle { font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; }
.chip-toggle input { accent-color: var(--ink); }

/* Summary table (fleet empty state in inspector) */
.summary { width: 100%; border-collapse: collapse; }
.summary tr { border-top: 1px solid var(--ink); }
.summary tr:last-child { border-bottom: 1px solid var(--ink); }
.summary td { padding: 10px 0; font: 13px/1.4 var(--sans); vertical-align: baseline; }
.summary td:first-child { font-weight: 500; }
.summary td:nth-child(2) { color: var(--mid); }
.summary td:last-child { text-align: right; font: 500 16px/1 var(--serif); }

/* KPIs (fuzzer view) */
.kpis { display: grid; gap: 22px 28px; margin-bottom: 28px; }
.kpis-1x5 { grid-template-columns: repeat(5, 1fr); }
.kpi { padding-top: 10px; border-top: 1px solid var(--ink); }
.kpi-lbl { font: 500 11px/1 var(--sans); letter-spacing: 0.14em; text-transform: uppercase; }
.kpi-num { font: 500 28px/1.05 var(--serif); letter-spacing: -0.01em; margin-top: 10px; }
.kpi-num.mono { font: 500 20px/1.1 var(--mono); letter-spacing: 0; }

/* Topology */
.topology-svg { width: 100%; max-width: 900px; height: auto; display: block; margin: 4px auto 12px; }
.topo-link { stroke: var(--ink); stroke-width: 1; }
.topo-link.active { stroke: var(--cobalt); }
.topo-node-circle { fill: var(--ink); stroke: var(--paper); stroke-width: 2; transition: fill 0.4s ease; }
.topo-node-circle.unreachable { fill: var(--red); }
.topo-node-circle.warn { fill: var(--mustard); }
.topo-node-circle.selected, .topo-node:hover .topo-node-circle { stroke: var(--ink); stroke-width: 3; }
.topo-node-label { font: italic 11px/1 var(--serif); fill: var(--ink); text-anchor: middle; dominant-baseline: hanging; pointer-events: none; }
.topo-node { cursor: pointer; }

/* Timeline */
.timeline { border-top: 1px solid var(--ink); max-height: calc(100vh - 280px); overflow-y: auto; position: relative; }
.timeline-row { display: grid; grid-template-columns: 120px 1px 1fr; align-items: baseline; border-bottom: 1px solid var(--rule); padding: 10px 0; }
.timeline-row.diverged { border-left: 2px solid var(--red); padding-left: 10px; }
.timeline-row.kb-focus { background: rgba(31,61,255,0.06); }
.timeline-seq { font: 500 18px/1 var(--serif); }
.timeline-rule { background: var(--ink); align-self: stretch; }
.timeline-meta { display: flex; gap: 14px; align-items: center; padding-left: 14px; font: 12px/1 var(--mono); color: var(--mid); }
.timeline-hash { color: var(--ink); }
.timeline-dots { margin-left: auto; display: flex; gap: 5px; }
.timeline-dots span { width: 6px; height: 6px; border-radius: 50%; background: var(--cobalt); cursor: pointer; transition: width 0.15s, height 0.15s; }
.timeline-dots span.diverged { background: var(--red); }
.timeline-dots span.emphasized { width: 8px; height: 8px; }
.new-chip { position: sticky; bottom: 8px; align-self: center; margin: 8px auto 0; font: 500 11px/1 var(--sans); letter-spacing: 0.08em; text-transform: uppercase; padding: 6px 10px; background: var(--ink); color: var(--paper); border: none; cursor: pointer; display: block; }
.new-chip[hidden] { display: none; }
.timeline-empty { padding: 36px 0; text-align: center; font: italic 12px/1.4 var(--serif); color: var(--mid); }

/* Fuzzer layers table */
.layers { width: 100%; border-collapse: collapse; margin-top: 24px; }
.layers thead th { text-align: left; font: 500 11px/1 var(--sans); letter-spacing: 0.14em; text-transform: uppercase; padding: 8px 0; border-bottom: 1px solid var(--ink); cursor: pointer; }
.layers tbody td { padding: 10px 0; font: 14px/1.3 var(--sans); border-bottom: 1px solid var(--rule); }
.layers tbody td:last-child { text-align: right; font: 500 18px/1 var(--serif); }
.layers tbody tr.crashed td:first-child { border-left: 2px solid var(--red); padding-left: 10px; }

/* Inspector */
.rail-inspector { grid-area: inspector; border-left: 1px solid var(--ink); display: flex; flex-direction: column; min-height: 0; }
.rail-inspector.collapsed { width: 24px; --inspector-w: 24px; }
.inspector-head { display: flex; align-items: center; justify-content: space-between; padding: 10px 16px; border-bottom: 1px solid var(--rule); }
.inspector-title { font: 500 14px/1 var(--serif); letter-spacing: -0.01em; }
.inspector-clear { background: none; border: none; font: 14px/1 var(--sans); cursor: pointer; width: 22px; height: 22px; color: var(--ink); }
.inspector-state { padding: 10px 16px; font: 12px/1.4 var(--sans); color: var(--mid); border-bottom: 1px solid var(--rule); min-height: 36px; user-select: text; }
.inspector-state b { font-weight: 500; color: var(--ink); }
.inspector-empty { padding: 12px 16px; }
.inspector-empty[hidden] { display: none; }
.inspector-logs-head { display: flex; align-items: center; gap: 10px; padding: 8px 16px; border-bottom: 1px solid var(--rule); flex-wrap: wrap; }
.inspector-logs-head[hidden] { display: none; }
.log-levels { display: flex; gap: 4px; }
.log-chip { background: none; border: 1px solid var(--rule); padding: 2px 6px; font: 500 10px/1 var(--sans); letter-spacing: 0.06em; text-transform: uppercase; cursor: pointer; color: var(--mid); }
.log-chip.active { border-color: var(--ink); color: var(--ink); }
.log-filter { flex: 1; min-width: 100px; border: none; border-bottom: 1px solid var(--rule); padding: 4px 0; font: 13px/1 var(--sans); background: transparent; outline: none; }
.inspector-logs { flex: 1; overflow-y: auto; overflow-x: auto; padding: 8px 16px 14px; font: 11px/1.55 var(--mono); white-space: pre; min-height: 0; }
.inspector-logs[hidden] { display: none; }
.inspector-logs .row { display: flex; gap: 14px; }
.inspector-logs .row.kb-focus { background: rgba(31,61,255,0.06); }
.inspector-logs .ts { color: var(--mid); }
.inspector-logs .lvl-ok { color: var(--cobalt); }
.inspector-logs .lvl-err { color: var(--red); }

/* Pane focus indicator */
[data-pane].pane-focus { box-shadow: inset 0 2px 0 0 var(--cobalt); }

/* Overlays (palette, cheatsheet) */
.overlay { position: fixed; inset: 0; background: rgba(10,10,10,0.4); display: flex; align-items: flex-start; justify-content: center; padding-top: 14vh; z-index: 100; }
.overlay[hidden] { display: none; }
.palette-card, .cheatsheet-card { background: var(--paper); border: 1px solid var(--ink); width: min(520px, 90vw); padding: 16px; }
.palette-input { width: 100%; border: none; border-bottom: 1px solid var(--ink); padding: 6px 0; font: 14px/1 var(--sans); outline: none; background: transparent; }
.palette-list { list-style: none; margin-top: 10px; max-height: 320px; overflow-y: auto; }
.palette-list li { padding: 6px 0; cursor: pointer; font: 13px/1.2 var(--sans); border-bottom: 1px solid var(--rule); }
.palette-list li.kb-focus { background: rgba(31,61,255,0.08); }
.cheatsheet-card h2 { font: 500 18px/1.2 var(--serif); margin-bottom: 12px; }
.cheatsheet-card dl { display: grid; grid-template-columns: 110px 1fr; gap: 6px 16px; font: 12px/1.4 var(--sans); }
.cheatsheet-card dt { font-family: var(--mono); color: var(--mid); }

/* Responsive */
@media (max-width: 1279px) {
  .rail-inspector { width: var(--inspector-w); }
}
@media (max-width: 1039px) {
  body { padding: 16px; }
  .workbench { grid-template-columns: 180px 1fr; grid-template-areas: "topbar topbar" "rail main"; }
  .rail-inspector { display: none; }
  .rail-inspector.open { display: flex; position: fixed; top: 16px; right: 16px; bottom: 16px; width: 320px; background: var(--paper); border: 1px solid var(--ink); z-index: 50; }
}
@media (max-width: 719px) {
  .workbench { grid-template-columns: 1fr; grid-template-areas: "topbar" "main"; }
  .rail-nodes { display: none; }
  .rail-nodes.open { display: flex; position: fixed; top: 16px; left: 16px; bottom: 16px; width: 220px; background: var(--paper); border: 1px solid var(--ink); z-index: 50; }
}
```

- [ ] **Step 3: Verify the page still loads with the old `app.js`**

Start the server (`node dashboard/server.js` won't run without `CONFIG_PATH`; instead open `index.html` via mock query against the existing fixtures by running it from the static dir).

Run from `xrpl-confluence/dashboard/static/`:

```bash
python3 -m http.server 8001
```

Open `http://localhost:8001/?mock=1` in a browser.

Expected: The new shell renders — three columns visible, top bar shows "CONNECTING" and `⌘K`. Console will show errors because `app.js` is looking for the old `#overview-kpis`, `.tab`, `.drawer*` IDs. That's fine for this task — we're verifying layout/markup, not behavior. Kill the server when done.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/index.html dashboard/static/style.css
git commit -m "dashboard: workbench shell (three-column grid, no JS rewire yet)"
```

---

## Task 2: Store, URL routing, and view switching

**Files:**
- Modify: `dashboard/static/app.js`

Introduce `createStore`, replace `setTab` with `setView`, parse and write `location.hash`. Existing renderers stay but we wire them to subscribe to the store. Selection and filters remain unused this task; we get view-switching working end-to-end first.

- [ ] **Step 1: Rewrite the top of `app.js` with store + URL routing**

Replace the entire contents of `dashboard/static/app.js` with the following. (We rewrite holistically because the file is small and the surgery would touch nearly every section anyway.)

```js
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

  // ── Renderers (placeholders — real bodies arrive in later tasks) ──
  // Existing renderers will be reinstated in Task 3 (rail), Task 4 (main views), Task 5 (inspector).

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
```

- [ ] **Step 2: Manual verification — view switching + URL**

From `xrpl-confluence/dashboard/static/`:

```bash
python3 -m http.server 8001
```

Open `http://localhost:8001/?mock=1`. Expected:

- Three segmented buttons at top of main: Timeline / Topology / Fuzzer.
- Clicking each changes the URL hash to `#/timeline`, `#/topology`, `#/fuzzer` and the corresponding view container shows (still empty bodies for now).
- Top bar sync pill becomes `CONNECTED` after the mock fetch.
- Pause button toggles `⏸ ↔ ▶`.
- Top bar context shows `· seq N · 0s ago` once mock data loads (it has `validated_ledger.seq`).

Kill the server.

- [ ] **Step 3: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: introduce store, URL routing, view switching"
```

---

## Task 3: Left rail — node list + filter + KPIs

**Files:**
- Modify: `dashboard/static/app.js`

Render the node list with health pips, wire the rail filter, populate the bottom KPIs. Clicking a row sets `selected` (we won't paint the inspector yet — that's Task 5 — but the URL will update and the selected style will appear).

- [ ] **Step 1: Add rail subscribers above the `// ── Init ──` line in `app.js`**

Insert (replacing the `// ── Renderers (placeholders …) ──` block) the following block:

```js
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
```

- [ ] **Step 2: Wire the rail filter input in the `DOMContentLoaded` block**

In `app.js`, inside the `document.addEventListener("DOMContentLoaded", () => { … })` block, after the existing wiring and before `applyHashToStore()`, add:

```js
    // Rail filter
    document.getElementById("rail-filter").addEventListener("input", (e) => {
      store.setFilter({ nodes: e.target.value });
    });
```

- [ ] **Step 3: Manual verification**

Start `python3 -m http.server 8001` in `dashboard/static/`, open `http://localhost:8001/?mock=1`. Expected:

- Left rail lists all mock nodes with a colored pip each.
- Typing in the rail filter narrows the list live.
- Clicking a row inverts it (white-on-black) and the URL hash gains `?node=<name>`.
- Bottom of rail shows five KPIs: Ledger / Converge / Spread / Diverge / Online with real numbers.
- Reloading the page with `?node=goxrpl-1` in the hash keeps that row selected.

Kill the server.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: rail node list, filter, and KPIs"
```

---

## Task 4: Main views — timeline, topology, fuzzer reinstated

**Files:**
- Modify: `dashboard/static/app.js`

Reinstate the timeline/topology/fuzzer renderers from the previous version, wired to the store. Add timeline filter + only-divergences toggle + per-dot click selection + divergence emphasis. Add fuzzer layer-name sort (default desc by count).

- [ ] **Step 1: Add view renderers above `// ── Init ──`**

Append to `app.js` (after the rail blocks added in Task 3, before `// ── Init ──`):

```js
  // ── Timeline state ──────────────────────────────────────────
  const MAX_TIMELINE = 80;
  const prevValidatedSeqs = {};
  const prevClosedSeqs = {};
  const closeRows = new Map(); // seq → { time, hash, byNode: Map }
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
          <div class="timeline-seq">${seq.toLocaleString("en-US")}</div>
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
```

- [ ] **Step 2: Wire timeline filter inputs + scroll, fuzzer header sort, in `DOMContentLoaded`**

Inside the `DOMContentLoaded` block, after the rail-filter wiring from Task 3 but **before** `applyHashToStore()`, add the event listeners:

```js
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
```

Then **after** the `applyHashToStore()` call in the init block, add the input-value reflection so it picks up URL-derived filters:

```js
    // Reflect URL filters back into inputs on load (runs after applyHashToStore)
    const initFilters = store.get().ui.filters;
    document.getElementById("timeline-filter").value = initFilters.timeline;
    document.getElementById("timeline-only-diverged").checked = initFilters.onlyDivergences;
```

- [ ] **Step 3: Manual verification**

Start the static server, open `http://localhost:8001/?mock=1`. Expected:

- Timeline view shows ledger rows from `mock.json` with seq, time, hash, and one dot per node.
- Switching to Topology shows the ring of node circles with hover ring; clicking one selects it.
- Switching to Fuzzer shows seed, KPIs, and a sorted layers table; clicking the "Divergences" header flips sort.
- Selecting a node makes its dots in Timeline grow visibly (8px vs 6px).
- Typing in the timeline filter narrows by seq/hash substring and updates the URL.
- Toggling "only divergences" hides rows where no node diverged (mock fixtures may have none; that's expected — empty state shows "No matches.").
- The Timeline segment badge shows `· N diverged` in red when any divergent rows exist.

Kill the server.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: reinstate timeline/topology/fuzzer views, wired to store"
```

---

## Task 5: Inspector — selection-driven state + logs, with filters

**Files:**
- Modify: `dashboard/static/app.js`

Populate the inspector when a node is selected; render the fleet-summary empty state otherwise. Wire the log level chips, log filter, follow-tail. Replace the old drawer poll.

- [ ] **Step 1: Add inspector subscribers and log polling**

Append to `app.js` after the fuzzer render block, before `// ── Init ──`:

```js
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
      stateEl.innerHTML = `<b>${ss.server_state || ss.status || "—"}</b> · ${seq} · peers ${ss.peers ?? "—"} · ${ss.build_version || ""}`;
    } else if (data === null) {
      stateEl.textContent = "Failed to fetch state.";
    } else {
      stateEl.textContent = "Loading…";
    }
  }

  function renderInspectorLogs(s, data) {
    const logsEl = document.getElementById("inspector-logs");
    const f = s.ui.filters;
    let entries = (data?.logs || []).slice().reverse();
    entries = entries.filter((e) => f.logLevels[levelOf(e)]);
    const q = f.logs.trim().toLowerCase();
    if (q) entries = entries.filter((e) => (e.message || "").toLowerCase().includes(q) || (e.level || "").toLowerCase().includes(q));
    if (!entries.length) {
      logsEl.innerHTML = `<div class="row" style="color:var(--mid)">No log entries.</div>`;
    } else {
      logsEl.innerHTML = entries.map((e) => {
        const t = (e.ts.split("T")[1] || e.ts).split(".")[0];
        const klass = `lvl-${levelOf(e)}`;
        return `<div class="row"><span class="ts">${t}</span><span class="${klass}">${e.level}</span><span>${e.message}</span></div>`;
      }).join("");
    }
    if (f.followTail) logsEl.scrollTop = logsEl.scrollHeight;
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
      if (s.ui.selected) startInspectorPolling(s.ui.selected); else stopInspectorPolling();
    }

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
```

- [ ] **Step 2: Wire inspector controls in `DOMContentLoaded`**

Append inside the `DOMContentLoaded` block, after the fuzzer-sort wiring from Task 4:

```js
    // Inspector clear
    document.getElementById("inspector-clear").addEventListener("click", () => setSelected(null));

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
```

- [ ] **Step 3: Manual verification**

Start the static server, open `http://localhost:8001/?mock=1`. Expected:

- With nothing selected, inspector header reads "Fleet" and the body shows the four-row summary (ledger spread / unreachable / divergences / crashes).
- Click a node row in the left rail → inspector header shows the node name, state line populates from mock state, log rows appear in mono.
- Toggle `err` chip off → red rows disappear; toggle back → they return.
- Type in the log filter → list narrows.
- Uncheck `follow` → adding/refreshing logs no longer auto-scrolls.
- Click ✕ in inspector header → selection clears, fleet summary returns.
- Reload with `#/timeline?node=goxrpl-1` → that node is selected and inspector populated.

Kill the server.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: inspector with selection-driven state, logs, level/text filters"
```

---

## Task 6: Keyboard navigation and pane focus

**Files:**
- Modify: `dashboard/static/app.js`

Add a single delegated `keydown` handler covering view switching, pane cycling, list navigation, filter focus, esc layering, pause, divergence jump, copy, and cheatsheet.

- [ ] **Step 1: Add keyboard handler before `// ── Init ──`**

Append to `app.js`:

```js
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
      if (store.get().ui.selected) { setSelected(null); return; }
      return;
    }
    if (isTypingTarget(e.target)) return;
    if (e.metaKey && e.key.toLowerCase() === "k") { e.preventDefault(); document.getElementById("palette").hidden = false; document.getElementById("palette-input").focus(); return; }
    if (e.ctrlKey && e.key.toLowerCase() === "k") { e.preventDefault(); document.getElementById("palette").hidden = false; document.getElementById("palette-input").focus(); return; }

    switch (e.key) {
      case "1": setView("timeline"); break;
      case "2": setView("topology"); break;
      case "3": setView("fuzzer"); break;
      case "j": case "J": walkList(+1); break;
      case "k": case "K": walkList(-1); break;
      case "Enter": activateFocused(); break;
      case "/": e.preventDefault(); focusFilterForActivePane(); break;
      case " ": e.preventDefault(); setPaused(!store.get().ui.paused); break;
      case "n": case "N": jumpNextDivergence(); break;
      case "c": case "C": copyFocused(); break;
      case "?": document.getElementById("cheatsheet").hidden = false; break;
      case "Tab":
        e.preventDefault();
        cyclePane(e.shiftKey ? -1 : +1);
        break;
    }
  });

  // Cheatsheet dismiss
  document.addEventListener("DOMContentLoaded", () => {
    document.getElementById("cheatsheet").addEventListener("click", (e) => {
      if (e.target.classList.contains("overlay")) e.currentTarget.hidden = true;
    });
  });

  // Pane focus visual
  store.subscribe((s) => {
    document.querySelectorAll("[data-pane]").forEach((el) => el.classList.toggle("pane-focus", el.dataset.pane === s.ui.activePane));
  });
```

- [ ] **Step 2: Manual verification**

Start `python3 -m http.server 8001`, open `http://localhost:8001/?mock=1`. Expected:

- Press `1` `2` `3` → main view switches.
- Press `tab` → active pane underline moves rail → main → inspector → rail.
- With rail focused, press `j`/`k` → a node row gets a cobalt left border. Press `enter` → that node is selected, inspector populates.
- Press `/` → corresponding pane's filter input focuses. Type to filter, press `esc` → input clears and blurs.
- Press `space` → pause/resume toggles, button glyph flips.
- Press `n` while on timeline (with mock data the fixture has no divergences, so nothing should move — confirm there's no error in console; create a divergence by editing `mock.json` to give two nodes different `validated_ledger.hash` at the same `seq` to verify).
- Press `?` → cheatsheet overlay. Click outside or press `esc` to close.
- Cmd-K / Ctrl-K opens the palette overlay (empty list is fine — Task 7).

Kill the server.

- [ ] **Step 3: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: keyboard nav, pane cycling, divergence jump, copy, cheatsheet"
```

---

## Task 7: Command palette

**Files:**
- Modify: `dashboard/static/app.js`

Wire the palette overlay opened in Task 6. Substring scoring; sources are nodes, view names, and pause/resume + copy state.

- [ ] **Step 1: Add palette wiring before `// ── Init ──`**

Append to `app.js`:

```js
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
```

- [ ] **Step 2: Manual verification**

Start the static server, open `http://localhost:8001/?mock=1`. Expected:

- Click `⌘K` button or press `Cmd-K` → palette opens with all nodes, three view entries, "Pause", "Copy state".
- Type `go` → list filters to nodes starting with `goxrpl-`.
- Arrow keys move focus row; Enter fires; Esc closes; clicking outside closes.
- "View: topology" entry switches view; "Pause" toggles pause; "Node: goxrpl-1" selects that node.

Kill the server.

- [ ] **Step 3: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: command palette (substring filter, keyboard-driven)"
```

---

## Task 8: Final cleanup, responsive QA, and end-to-end smoke

**Files:**
- Modify: `dashboard/static/app.js` (only if smoke uncovers issues)
- Modify: `dashboard/static/style.css` (only if smoke uncovers issues)

- [ ] **Step 1: Smoke against the live server fixtures**

From the project root:

```bash
cd xrpl-confluence/dashboard
CONFIG_PATH=$PWD/../scripts/config.example.json node server.js
```

(If `scripts/config.example.json` doesn't exist, fall back to the static-server mock run as in earlier tasks — the smoke matters for the rendering, not for the live data path.)

Open `http://localhost:8080/` (or the static-server URL with `?mock=1`).

Walk this checklist by clicking and keyboard alone — no hand-holding:

- [ ] Cold load → URL gets normalized to `#/timeline`.
- [ ] Click each segmented view; URL updates each time.
- [ ] Select a node from rail → URL gets `?node=…` and inspector populates.
- [ ] Refresh → selection persists.
- [ ] Edit hash to `#/topology?node=goxrpl-2` → load shows topology with that node ringed and inspector for it.
- [ ] Type `/` in rail → focused; type filter; Esc → blurs and clears.
- [ ] Pause → +N chip starts accumulating after the next poll; resume flushes.
- [ ] Toggle log level chips off/on → log rows respect them.
- [ ] `tab` cycles panes; `j/k` walks the active list.
- [ ] `?` → cheatsheet; `Esc` closes; `⌘K` → palette; type, arrow, enter.

- [ ] **Step 2: Responsive QA**

Resize the browser to 1280px, 1100px, 900px, 600px widths.

- ≥1280: three columns visible.
- 1040–1279: three columns, inspector unchanged.
- 720–1039: rail + main only (no inspector by default). Selecting a node should still update the URL even though the inspector isn't visible — verify by widening back. (Mobile sheet behavior is out of scope for v1; just confirm no overflow or broken layout.)
- <720: single-column main only; rail and inspector hidden. Confirm the top bar wraps cleanly without overflow.

- [ ] **Step 3: Lint pass — verify no console errors during a one-minute idle**

Open the dev tools console, open the dashboard against mock for 60 seconds while flipping views and selections. Expected: zero errors, zero unhandled promise rejections.

If any appear, fix them in the relevant task's file; commit with `fix(dashboard): …` and continue.

- [ ] **Step 4: Final commit**

If any fixes landed in Step 1–3, commit them now. Otherwise, no-op.

```bash
git status
# if dirty:
git add dashboard/static/
git commit -m "dashboard: smoke fixes"
```

---

## Coverage check (self-review)

Spec sections mapped to tasks:

- §1.1 Top bar → Task 1 (markup/styles) + Task 2 (sync, ctx subscribers, pause)
- §1.2 Left rail → Task 1 (shell) + Task 3 (list, filter, KPIs)
- §1.3 Main view → Task 1 (shell, segmented) + Task 4 (renderers, filters, sort, badges)
- §1.4 Inspector rail → Task 1 (shell) + Task 5 (state, logs, level/text filters, follow-tail, empty state)
- §2 State (store/URL/pause) → Task 2
- §3.1 Selection semantics → Task 3 (rail click), Task 4 (topology + per-dot click + emphasis), Task 5 (poll start/stop)
- §3.2 Keyboard → Task 6
- §3.3 Filters → Tasks 3 (rail), 4 (timeline), 5 (logs), 4 (fuzzer sort)
- §3.4 Divergence salience → Task 4 (badge, border, dot emphasis) + Task 6 (`n` jump)
- §3.5 Command palette → Task 7
- §4 Files → all tasks confined to the three files
- §5 Responsive → Task 1 (media queries) + Task 8 (QA)
- §7 Test plan → Task 8

Out-of-scope items (multi-node compare, time scrubbing, WS log streaming, server-side filtering, auth) — not implemented, as the spec specifies.

