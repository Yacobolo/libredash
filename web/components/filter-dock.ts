const filterDockStorageKey = 'libredash:filters-open'
const dockSelector = 'details[data-filter-dock]'

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

function setDockOpen(dock: HTMLDetailsElement, open: boolean): void {
  dock.open = open
  syncDockClasses(dock, open)
  storeOpen(open)
}

function syncDock(dock: HTMLDetailsElement, open: boolean): void {
  if (dock.open !== open) {
    dock.open = open
  }
  syncDockClasses(dock, open)
}

function syncDockClasses(dock: HTMLDetailsElement, open: boolean): void {
  dock.dataset.state = open ? 'open' : 'closed'
  dock.classList.toggle('sm:w-filter-dock', open)
  dock.classList.toggle('sm:w-filter-closed', !open)
  dock.classList.toggle('bg-app', open)
  dock.classList.toggle('bg-panel-muted', !open)

  const summary = dock.querySelector<HTMLElement>('[data-filter-summary]')
  summary?.classList.toggle('sm:hidden', open)
  summary?.classList.toggle('sm:flex', !open)

  const pane = dock.querySelector<HTMLElement>('[data-filter-pane]')
  pane?.classList.toggle('sm:block', open)
  pane?.classList.toggle('sm:hidden', !open)
}

function hydrateFilterDock(dock: HTMLDetailsElement): void {
  syncDock(dock, storedOpen())
  dock.dataset.ready = 'true'
  dock.addEventListener('toggle', () => {
    storeOpen(dock.open)
    syncDockClasses(dock, dock.open)
  })
  dock.addEventListener('ld-filters-close', () => setDockOpen(dock, false))
}

function hydrateFilterDocks(): void {
  document.querySelectorAll<HTMLDetailsElement>(dockSelector).forEach(hydrateFilterDock)
}

window.addEventListener('storage', (event) => {
  if (event.key !== filterDockStorageKey) return
  const open = event.newValue === 'open'
  document.querySelectorAll<HTMLDetailsElement>(dockSelector).forEach((dock) => syncDock(dock, open))
})

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', hydrateFilterDocks, { once: true })
} else {
  hydrateFilterDocks()
}
