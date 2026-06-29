import { defineElementOnce } from '../shared/lazy-registry'

type DashboardComponentKind =
  | 'ld-filter-card'
  | 'ld-kpi-card'
  | 'ld-echart'
  | 'ld-data-table'
  | string

const dashboardComponentLoaders: Record<string, () => Promise<unknown>> = {
  'ld-filter-panel': () => import('./filters/filter-panel'),
  'ld-filter-card': () => import('./filters/filter-card'),
  'ld-kpi-card': () => import('./charts/echart'),
  'ld-echart': () => import('./charts/echart'),
  'ld-data-table': () => import('./table/data-table'),
}

export function loadDashboardComponent(kind: DashboardComponentKind): Promise<void> {
  const loader = dashboardComponentLoaders[kind]
  if (!loader) return Promise.reject(new Error(`unsupported dashboard component kind ${kind}`))
  return defineElementOnce(kind, loader)
}

export function dashboardComponentKinds(): string[] {
  return Object.keys(dashboardComponentLoaders)
}
