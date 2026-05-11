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
