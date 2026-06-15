// web/components/filter-dock.ts
var filterDockStorageKey = "libredash:filters-open";
function storedOpen() {
  try {
    return localStorage.getItem(filterDockStorageKey) === "open";
  } catch {
    return false;
  }
}
function storeOpen(open) {
  try {
    localStorage.setItem(filterDockStorageKey, open ? "open" : "closed");
  } catch {
  }
}
function syncDock(dock, open) {
  if (dock.open !== open) {
    dock.open = open;
  }
  dock.dataset.state = open ? "open" : "closed";
}
function hydrateFilterDock(dock) {
  syncDock(dock, storedOpen());
  dock.dataset.ready = "true";
  dock.addEventListener("toggle", () => {
    storeOpen(dock.open);
    dock.dataset.state = dock.open ? "open" : "closed";
  });
}
function hydrateFilterDocks() {
  document.querySelectorAll("details.filters-dock").forEach(hydrateFilterDock);
}
window.addEventListener("storage", (event) => {
  if (event.key !== filterDockStorageKey) return;
  const open = event.newValue === "open";
  document.querySelectorAll("details.filters-dock").forEach((dock) => syncDock(dock, open));
});
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", hydrateFilterDocks, { once: true });
} else {
  hydrateFilterDocks();
}
