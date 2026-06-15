const filterDockStorageKey = 'libredash:filters-open'

function storedOpen(): boolean {
  try {
    return localStorage.getItem(filterDockStorageKey) === 'open'
  } catch {
    return false
  }
}

function storeOpen(open: boolean): void {
  try {
    localStorage.setItem(filterDockStorageKey, open ? 'open' : 'closed')
  } catch {
    // Ignore storage failures; the details element state still updates.
  }
}

function syncDock(dock: HTMLDetailsElement, open: boolean): void {
  if (dock.open !== open) {
    dock.open = open
  }
  dock.dataset.state = open ? 'open' : 'closed'
}

function hydrateFilterDock(dock: HTMLDetailsElement): void {
  syncDock(dock, storedOpen())
  dock.dataset.ready = 'true'
  dock.addEventListener('toggle', () => {
    storeOpen(dock.open)
    dock.dataset.state = dock.open ? 'open' : 'closed'
  })
}

function hydrateFilterDocks(): void {
  document.querySelectorAll<HTMLDetailsElement>('details.filters-dock').forEach(hydrateFilterDock)
}

window.addEventListener('storage', (event) => {
  if (event.key !== filterDockStorageKey) return
  const open = event.newValue === 'open'
  document.querySelectorAll<HTMLDetailsElement>('details.filters-dock').forEach((dock) => syncDock(dock, open))
})

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', hydrateFilterDocks, { once: true })
} else {
  hydrateFilterDocks()
}
