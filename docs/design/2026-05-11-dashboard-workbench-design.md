# Confluence Dashboard — Workbench Mode

**Status**: Approved design, not yet implemented
**Date**: 2026-05-11
**Scope**: `xrpl-confluence/dashboard/static/{index.html,style.css,app.js}`
**Builds on**: `2026-05-11-dashboard-ui-rework-design.md` (the poster style and palette stay; this design changes the interaction model on top of it)

## Intent

The current dashboard ships the editorial poster style: paper card, serif headlines, cobalt/mustard/red health palette, five tabs (`Overview · Nodes · Topology · Timeline · Fuzzer`). The style is good. The interaction model is not: it forces tab-switching, has no persistent selection, no filtering, no keyboard nav, and a single-slot drawer that disappears your selection when you change views.

Rework the interior into a single-pane *workbench* with three persistent columns. Selection is global and survives view changes. Filtering, keyboard nav, deep links, and pause/play replace clicking through tabs. Visual style is preserved exactly — only layout, state, and inputs change.

## Decisions (locked during brainstorming)

1. **Three persistent columns** in one paper sheet: left rail (nodes + KPIs), main view (one of Timeline / Topology / Fuzzer), inspector rail (selected-node state + logs). Hairline `--ink` rules between them.
2. **Overview tab is removed.** Its content moves to the left-rail KPIs (always visible) and to the inspector's fleet-summary fallback when nothing is selected.
3. **Drawer is gone.** Inspector replaces it — always-on, collapsible to a thin gutter, never modal.
4. **URL is the source of truth** for `view`, `selected node`, and key filters; deep-linkable and back-button-safe.
5. **Single-select only for v1.** Multi-node compare is v2.
6. **Pause/play** freezes UI rendering only; SSE keeps arriving and the next un-pause renders what buffered.
7. **Style untouched.** Palette, type ramp, hairline borders, paper background — all preserved.

## 1. Layout

```
┌──────────────────────────────────────────────────────────────────────┐
│  XRPL Confluence            [● CONNECTED]  [⏸ paused]  ⌘K  · seq 19  │
├──────────────┬───────────────────────────────────────┬───────────────┤
│  NODES       │  ◯ Timeline  ◯ Topology  ◯ Fuzzer     │  goxrpl-2     │
│  ──────────  │  ──────────────────────────────────   │  ──────────   │
│  / filter    │                                       │  full · #19   │
│  ● goxrpl-1  │   [ main view body ]                  │  4 peers      │
│  ● goxrpl-2◄ │                                       │  build 2.4.0  │
│  ● goxrpl-3  │                                       │               │
│  ● rippled-1 │                                       │  ── logs ──   │
│  ● rippled-2 │                                       │  / filter     │
│              │                                       │  [ok][warn][err]│
│  ──────────  │                                       │  10:21 ok …   │
│  ledger 19   │                                       │  10:20 ok …   │
│  converge 3s │                                       │  follow▣      │
│  diverge 0   │                                       │               │
└──────────────┴───────────────────────────────────────┴───────────────┘
```

### 1.1 Top bar

Replaces the current `.tabs` strip. Lives at the top of the paper card, one hairline rule below.

- Wordmark `XRPL Confluence` on the left (serif, 14px, weight 500).
- Sync pill in the middle-right (existing `.sync` styles, unchanged).
- Pause/play button — icon-only, 22px square, no border, toggles `state.ui.paused`. While paused, a small inline chip reads `+N updates pending` (same visual as the existing `.new-chip`, inline).
- `⌘K` affordance — small mono chip; clicking opens a minimal command palette overlay (see §3.5).
- Live context — `· seq 19,402,118 · 3s ago` in italic serif, combining today's `#footer-ctx` and `#footer-age`. The footer is removed; both pieces live here.

### 1.2 Left rail (Nodes)

Width: 220px desktop, collapses to a 44px icon strip below 1040px, stacks to top above 640px.

- Header label `NODES` (uppercase sans, 11px, letter-spacing 0.14em) plus a one-line filter input directly under it (`/` to focus).
- Node list — one row per node, ~32px tall:
  - 8px colored health pip (existing `--cobalt` / `--mustard` / `--red` rules from `healthClass()`).
  - Node name in 13px sans.
  - Selected row: full-row inverted (`--ink` background, `--paper` text), matching active-tab styling.
- Hairline rule, then **fleet KPIs** pinned to the bottom of the rail:
  - `LEDGER` / `CONVERGE` / `DIVERGE` / `ONLINE` — same numbers the Overview tab shows today, in the same `.kpi` styling but one-per-row (stacked) instead of `kpis-2x2`.

### 1.3 Main view

The working canvas. One view at a time, switched by a small segmented control under the main column header.

- Segmented control — three pill buttons `Timeline / Topology / Fuzzer`, hairline border, active pill inverted. Replaces the full-width `.tabs`.
- Each pill shows a count badge when relevant: `Timeline · 2 diverged`, `Fuzzer · 3 crashes`. Badge uses `--red` when nonzero, hidden when zero.
- View body keeps the existing renderer code (`renderTimeline`, `renderTopology`, `renderFuzzer`) — only the surrounding chrome changes.

### 1.4 Inspector rail

Width: 320px desktop, collapsible to a 24px gutter via a chevron on its left edge (replaces today's `#drawer-resize` handle behavior). State persists across reloads in `localStorage`.

- Header — selected node name in serif 18px, with a small `✕` to clear selection.
- **State block** — one line, exactly today's drawer-state content (`<b>state</b> · validated #N · peers N · build …`), but always visible.
- Hairline rule.
- **Logs block** — header row with three level chips (`ok` / `warn` / `err`, toggle to filter), text filter input, follow-tail checkbox. Below: scrollable mono log list, same rendering as `drawer-logs` today.
- **Empty state** (no node selected) — replaces the inspector content with the existing Overview summary table (`#overview-summary`) so the right rail always carries useful info.

## 2. State

### 2.1 Shape

Replaces the implicit module-level vars (`latest`, `latestFuzz`, `drawerName`, `timelinePinned`, etc.) with one object:

```js
const state = {
  nodes: [],              // from /api/nodes or SSE
  fuzz: null,             // from /api/fuzz
  logs: {},               // { [nodeName]: { state, logs } } — lazy, only fetched for selected
  ui: {
    view: "timeline",     // "timeline" | "topology" | "fuzzer"
    selected: null,       // node name or null
    paused: false,
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
};
```

### 2.2 Store

A 40-line subscribe/notify replacing the current `const renderers = []` array:

```js
function createStore(initial) {
  let s = initial;
  const subs = new Set();
  return {
    get: () => s,
    set: (patch) => { s = { ...s, ...patch }; subs.forEach((fn) => fn(s)); },
    setUI: (patch) => { s = { ...s, ui: { ...s.ui, ...patch } }; subs.forEach((fn) => fn(s)); },
    subscribe: (fn) => { subs.add(fn); return () => subs.delete(fn); },
  };
}
```

Each panel (left rail, main view, inspector, top bar) subscribes independently. Per-panel `render(prev, next)` compares the slices it cares about (selection-only changes don't repaint the timeline body, etc.).

### 2.3 URL

`location.hash` is the source of truth for routable UI state:

```
#/timeline?node=goxrpl-2&only=diverged
#/topology?node=goxrpl-2
#/fuzzer
```

- On load: parse hash → seed `state.ui.{view, selected, filters.onlyDivergences}`.
- On `state.set/setUI`: if routable fields changed, `history.replaceState` with the new hash. (Use `replaceState`, not push — back button should escape the app, not walk filter changes.)
- `hashchange` listener (for manual edits / shared links) re-applies hash → state.

### 2.4 Pause/play

- `state.ui.paused === true` → renderers receive updates to `state.nodes` / `state.fuzz` but do not repaint. A `pendingUpdates` counter increments each suppressed render.
- Top-bar chip shows `+N pending` while paused.
- On unpause: one full render flushes; counter resets.
- Selection-changes and view-changes always render, even while paused (user-initiated intent overrides).

## 3. Interaction

### 3.1 Selection semantics

| Trigger | Behavior |
|---|---|
| Click row in left rail | `selected = name`; no view change |
| Click circle in Topology | `selected = name`; no view change |
| Click a colored dot on a Timeline row | `selected = name of that node`; no view change |
| Click Timeline row body | no-op (selection is dot-scoped, intentional) |
| `esc` with selection set | `selected = null` |
| `esc` with no selection but inspector expanded | collapse inspector |

When `selected` is set:
- Left rail row gets inverted style.
- Topology circle gets the existing `.selected` stroke ring.
- Timeline emphasizes that node's column: its dot grows from 6px → 8px on every row, and rows where it diverged get the 2px `--red` left border described in §3.4. (No row filtering — every row carries every node, so filtering by node would empty almost nothing.)
- Inspector populates from `/api/logs/<name>` polled every 2s (same as today's `drawerPoll`). Polling is per-selection; clearing selection stops it.

### 3.2 Keyboard

| Key | Action |
|---|---|
| `1` `2` `3` | Switch view: timeline / topology / fuzzer |
| `j` `k` | Walk the focused list (nodes rail, or timeline rows when main has focus) |
| `enter` | Select focused row (rail) or toggle follow-tail (logs) |
| `/` | Focus the filter input for the focused pane |
| `esc` | Clear filter if filled → else clear selection → else collapse inspector |
| `space` | Toggle pause |
| `n` | Jump to next divergence (Timeline only) |
| `c` | Copy: selected log row, or inspector state line, or focused timeline row |
| `?` | Toggle cheatsheet overlay (small, dismissable with `esc`) |
| `tab` / `shift+tab` | Cycle panes (rail → main → inspector) |

Focus model: at any moment exactly one of `rail | main | inspector` is the *active pane*. A 2px cobalt underline on that pane's header indicates it. `tab` cycles. Keys that are pane-scoped (`j/k`, `/`, `n`) operate on the active pane.

Keys are suppressed while a text input has DOM focus, except `esc` which always blurs first.

### 3.3 Filters

- **Nodes rail** — substring match on `n.name`. Ephemeral (not in URL).
- **Timeline** — text filter on seq + hash; `only divergences` toggle; both persisted in URL (`?q=...&only=diverged`). The `n` shortcut walks `closeRows` finding the next entry where any `byNode` value is `"diverged"`, scrolls it into view, and highlights the row briefly.
- **Logs** — three level chips and a text filter. Levels currently mapped: `error|unreachable → err`, `proposing|validating → ok`, everything else → `warn`. Filter is reset on selection change. Follow-tail toggle replaces today's unconditional `scrollTop = scrollHeight` (only auto-scrolls when checked).
- **Fuzzer** — sort by divergence count desc by default; click a header to flip; layer-name substring filter.

### 3.4 Divergence salience

Timeline divergence is currently easy to miss (6px red dots inline). Additions:

- View pill shows count: `Timeline · 2 diverged` in `--red`.
- A `↓ next divergence` chip floats over the Timeline body when any divergence is in the current list, mirroring the existing `.new-chip` styling. `n` triggers the same scroll.
- Divergent rows get a 2px `--red` left border (same idiom as `.layers tbody tr.crashed`).

### 3.5 Command palette

`⌘K` (or `ctrl+K`) opens a minimal overlay — paper card centered, ink hairline, one input plus a flat result list. Sources:

- `Node: goxrpl-2` → select that node.
- `View: timeline / topology / fuzzer` → switch view.
- `Pause` / `Resume`.
- `Copy state` → copies the inspector state line.
- `Open: github` / `Open: grafana` (stretch — only if env exposes the URLs).

Keyboard: `↑↓` to walk, `enter` to fire, `esc` to dismiss. No fuzzy lib — substring scoring is enough.

## 4. Components & files

All work stays in three files:

- `dashboard/static/index.html` — restructure body into the three-column grid; remove `.tabs`, `.drawer`, `#overview-summary`, `.footer`; add `#topbar`, `#rail-nodes`, `#main-view`, `#main-switch`, `#inspector`, `#palette`.
- `dashboard/static/style.css` — keep `:root`, type ramp, palette, KPI/summary/timeline/fuzzer styles. Replace `.poster` (column flex) with a grid layout `220px 1fr 320px`. Add styles for the new top bar, segmented control, inspector chips, palette overlay. Drop `.drawer`, `.tab`, `.tabs`, `.footer` (their elements are gone).
- `dashboard/static/app.js` — refactor:
  - Introduce `createStore` and migrate `latest` / `latestFuzz` / drawer state into it.
  - Replace `renderers.push(fn)` with per-panel `subscribe` calls.
  - Replace `setTab` with `setView`; replace `openDrawer/closeDrawer` with `setSelected(name)`.
  - Add `parseHash` / `pushHash`; replace existing `hashchange` listener.
  - Add keyboard handler (one delegated `keydown` on `document`).
  - Add palette wiring.
  - Reuse `renderTimeline` / `renderTopology` / `renderFuzzer` bodies; only their containers and selection styling change.

No new dependencies. No build step introduced (matches current vanilla setup).

## 5. Responsive

| Width | Layout |
|---|---|
| ≥ 1280px | Three columns as drawn. |
| 1040–1279px | Three columns, inspector collapsible default-collapsed. |
| 720–1039px | Two columns: rail (180px) + main; inspector becomes a right-edge sheet that overlays main when opened. |
| < 720px | Single column, stacked: top bar → main → rail-as-bottom-sheet (via a `Nodes` button) → inspector-as-modal. |

Drop the current `@media print` block; the workbench isn't a print artifact. (The previous design's poster mode lived for that; this one doesn't.)

## 6. Out of scope (v2)

- Multi-node compare (two inspectors side by side).
- Time scrubbing / historical replay of ledger closes.
- WebSocket log streaming (logs still poll at 2s, matching today).
- Server-side filtering — all filters are client-side over already-fetched data.
- Auth / per-user settings — `localStorage` is the only persistence.

## 7. Test plan

- Manual: load with `?mock=1`, walk through every keyboard shortcut, every filter, deep-link via hash, reload mid-session and confirm selection + view + filters restore.
- Visual diff: capture screenshots of each view in both selection states and confirm style ramp matches the existing rework spec (palette, fonts, hairline widths).
- Regression: existing endpoints (`/api/nodes`, `/api/fuzz`, `/api/logs/:name`, `/events`, `/fixtures/*`) unchanged — server side untouched.
