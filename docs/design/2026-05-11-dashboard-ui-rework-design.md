# Confluence Dashboard UI Rework

**Status**: Approved design, not yet implemented
**Date**: 2026-05-11
**Scope**: `xrpl-confluence/dashboard/static/` (vanilla HTML/CSS/JS, served by `dashboard/server.js`)

## Intent

Rework the XRPL Confluence dashboard from its current functional-but-busy layout into a single calm "poster" interface, inspired by the GSMG.IO puzzle reference: deep dark canvas, one floating high-contrast white card holding all content. The dashboard remains a real-time interop monitor — every existing data feed is preserved — but the framing is editorial rather than ops-console.

## Decisions (locked during brainstorming)

1. **Dark canvas, floating white "poster"** as the only top-level layout idiom.
2. **Single white card with tabs at top** — no sidebar, no multi-poster grid. Tabs: `Overview · Nodes · Topology · Timeline · Fuzzer`.
3. **Editorial serif personality** — display serif headings, sans labels, italic subtitles, mono for raw IDs/numerics.
4. **Topology tab**: topology SVG occupies the full poster width; per-node logs slide in below as an in-card drawer when a node is clicked.

## 1. Shell

- Body background `#0a0a0a`, no header bar, no global chrome.
- One white card per page load, centered horizontally, ~96px gutter top and bottom of viewport.
- Card surface `#fcfbf7` (paper white), 1px solid `#0a0a0a` border, no shadow, no rounded corners.
- Card `max-width` is tab-dependent:
  - Overview / Timeline / Fuzzer → 720px
  - Nodes / Topology → 960px
- Tab bar lives inside the card, flush with the top edge:
  - 5 tabs, each `padding: 8px 14px 10px`, separated by 1px vertical rules (`#0a0a0a`).
  - Active tab inverts to white-on-black.
  - Tab labels: Inter, 11px, uppercase, letter-spacing 0.08em.
- Top-right corner of the tab bar: **sync status pill** = a small 8px dot + uppercase sans label (`CONNECTED` / `RECONNECTING` / `OFFLINE`). Replaces today's standalone `#sync-badge`.
- Card footer: 1px black rule, then a single italic Georgia line below it — `Last update · Ns ago · <tab-specific context>` (e.g. `· seed #f3a1c8` on the Fuzzer tab, `· ledger 8,421,902` on Timeline).

## 2. Typography

| Role | Family stack | Size | Notes |
|---|---|---|---|
| Tab title | `GT Sectra, Tiempos Headline, Georgia, serif` | 22–32px, weight 500 | Letter-spacing −0.01em. Always ends with a period (`Network Overview.`, `Topology.`). |
| Display number (KPI) | same serif stack | 30–36px, weight 500 | Letter-spacing −0.01em. Used for headline KPI counts. |
| Subtitle / caption | `Georgia, serif`, italic | 12–14px | Color `#888`. |
| Label | `Inter, system-ui, sans-serif` | 11px | Uppercase, letter-spacing 0.14em, color `#0a0a0a`. |
| Tab nav | `Inter, system-ui, sans-serif` | 11px | Uppercase, letter-spacing 0.08em. |
| Mono (IDs, hashes, logs, seeds) | `JetBrains Mono, SF Mono, monospace` | 11–12px | |

## 3. Color system

- Ink `#0a0a0a` (text, borders, ticks).
- Paper `#fcfbf7` (card surface).
- Canvas `#0a0a0a` (page background).
- Mid-gray `#888` (italic subtitles, footer timestamp).
- Rule-gray `#d0d0d0` (faint internal dividers when a 1px black rule would be too loud).

Three semantic accents, used **only** as 8px dots, 2px left/top rules, or text color — never as fills, never as backgrounds:

| Accent | Hex | Meaning |
|---|---|---|
| Cobalt | `#1f3dff` | Healthy / synced / current / quorum link. |
| Mustard | `#d4a800` | Lag / warning / divergent-but-recovering. |
| Signal red | `#c8201a` | Divergence / node down / fuzzer crash. |

No greens, no gradients, no drop shadows. Status color transitions crossfade over 400ms.

## 4. Per-tab layouts

### 4.1 Overview (720px)
- Title `Network Overview.` + italic subtitle (`Live interop status across goXRPL ↔ rippled`).
- 2×2 KPI grid: **Nodes online · Latest ledger · Proposers · Converge time**.
  - Each KPI: 1px black top rule, uppercase sans label below the rule, large serif number underneath.
- Below the grid, a 4-row **amendment / divergence summary table**:
  - Columns: `Item` (sans label), `Status` (sans label), `Count` (serif numeric).
  - Replaces the loose facts currently scattered across the consensus section.

### 4.2 Nodes (960px)
- Title `Nodes.` + italic subtitle (`Click any card to inspect.`).
- Grid of node cards: 3 columns at 960px, 2 columns at narrow viewports.
- Each card:
  - 2px top rule colored by node health (cobalt / mustard / red).
  - Implementation name (e.g. `goxrpl-0`) in serif, 18px, weight 500.
  - Peer ID in mono, 11px, color `#888`, truncated with ellipsis.
  - Mini KPI line below: `LEDGER · PEERS · LAG`, each value in mono, label in sans 10px uppercase.
- Clicking a card opens the **node log drawer** (same mechanism as Topology) anchored to the bottom of the poster.

### 4.3 Topology (960px)
- Title `Topology.` + italic subtitle (`Click a node to inspect its logs.`).
- Full-width SVG (~520px tall) for the trust graph:
  - Nodes: 14px circles, fill `#0a0a0a`, stroke none.
  - Edges: 1px solid `#0a0a0a` for plain peer links, 1px cobalt for validator quorum links.
  - Hover state: 2px black ring around node, label appears below the node in italic Georgia 11px.
- **In-card drawer** below the SVG:
  - Closed by default. Opens on node click, closes on the ✕ button.
  - 1px black top border, 28px sans header `goxrpl-0 · LOGS`, ✕ on the right.
  - Body: live log stream as mono 11px lines, monospace timestamps, log lines never wrap (overflow-x scroll if needed).
  - Resizable vertically via a 4px drag handle on the top border. Default height ≈ 30% of card height.

### 4.4 Timeline (720px)
- Title `Ledger Timeline.` + italic subtitle (`Latest closes, newest at bottom.`).
- Vertical sequence of close entries. Each entry is a single row:
  - Left column (~120px): serif ledger number (e.g. `8,421,902`).
  - 1px vertical rule.
  - Right column: close time in mono, set-hash in mono (truncated to 12 chars + ellipsis), one status dot per node (cobalt = matched on this close, red = diverged). Dot count = current node count.
- New closes append at the bottom; if the user has scrolled away from the bottom, position is pinned and a `↓ N new` chip appears above the footer.

### 4.5 Fuzzer (720px)
- Title `Fuzzer.` + italic subtitle (`Seed #<seedhash>` in mono).
- Top: 5 KPIs in a single row — **Submitted · Applied · Divergences · Crashes · Seed**.
  - Same KPI block style as Overview (top rule + sans label + serif number). Seed value renders in mono.
- Below: the `divergences by layer` table:
  - Sans column headers (`LAYER · DIVERGENCES`) with 1px black bottom rule.
  - Serif numeric counts.
  - Rows with `crashes > 0` get a 2px red left border.

## 5. Real-time behavior

- **All data refreshes are silent.** No pulses, no flashes, no re-fade-ins.
- The only visible "liveness" signal is the footer line `Last update · Ns ago`, refreshing every second.
- Append-on-arrival semantics for Timeline and node-drawer Logs: new rows append at the bottom; if user scroll is not at the bottom, position is pinned and a `↓ N new` chip is shown.
- Status color transitions (e.g. node cobalt → red) crossfade over 400ms.
- Failed refreshes do not show a spinner; they flip the sync badge to `RECONNECTING` (mustard) or `OFFLINE` (red).

## 6. Motion

| Event | Animation | Duration |
|---|---|---|
| Tab switch | Instant content swap | 0 |
| Drawer open / close | Ease-out slide from the bottom edge of the SVG/card | 180ms |
| Status color change | Crossfade | 400ms |

That is the entire motion budget. No spinners on data refresh.

## 7. Responsive

- **Breakpoint 1040px** — 960px posters (Topology, Nodes) clamp to `100vw − 32px` gutter. Tab bar becomes a horizontal scroll strip with 24px edge fades.
- **Breakpoint 640px** — Overview KPI grid collapses 2×2 → 1×4 (single column). Topology SVG keeps aspect ratio, drawer defaults to 50% card height. Nodes grid collapses to 1 column.
- **Print stylesheet** — drops the dark canvas, hides the sync badge and footer timestamp, keeps the paper card as the printable surface. Works as a free "export view as PDF report" mode.

## 8. What is preserved vs deleted

**Preserved**
- All five sections (Overview, Nodes, Topology, Timeline, Fuzzer) with their existing data feeds.
- Per-node log inspection.
- Topology graph (trust links, quorum highlighting).
- Fuzzer KPIs and divergence-by-layer table.

**Deleted**
- Loading spinners (`.loading-spinner`, "Connecting to nodes…" state). Replaced by `OFFLINE` sync badge + empty card.
- The standalone `#sync-badge` element (folded into the tab bar).
- The `(click to inspect)` hint (the drawer mechanic makes it self-evident).
- Gradients, drop shadows, rounded corners — none survive.

## 9. Implementation surface

Three files in `xrpl-confluence/dashboard/static/`:
- `index.html` — reorganized into a single `<main class="poster">` with a tab bar and 5 tab panels.
- `style.css` — full rewrite. Replace all current rules with the variables + typography + tab/poster system above. No framework. CSS-only theming.
- `app.js` — internal logic mostly preserved (SSE/polling, parsing). Render targets change:
  - Tab routing (hash-based: `#overview`, `#nodes`, `#topology`, `#timeline`, `#fuzzer`).
  - Drawer open/close + drag-resize.
  - `↓ N new` chip for Timeline / Logs.
  - Footer timestamp tick (1Hz).
- `server.js` (`xrpl-confluence/dashboard/server.js`) — **untouched**. No backend changes.

## 10. Non-goals

- No framework introduction (React/Vue/Svelte). Stays vanilla.
- No new data sources. Frontend-only rework against today's API surface.
- No mobile-first redesign. Responsive breakpoints exist for sanity, not for primary mobile use.
- No theming system / user-toggleable light mode. The aesthetic is the aesthetic.
