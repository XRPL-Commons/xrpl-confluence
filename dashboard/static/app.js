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
