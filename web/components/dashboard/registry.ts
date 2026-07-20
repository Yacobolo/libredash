import { defineElementOnce } from '../shared/lazy-registry'

type DashboardComponentKind =
  | 'lv-filter-card'
  | 'lv-kpi-card'
  | 'lv-echart'
  | 'lv-report-table'
  | string

const dashboardComponentLoaders: Record<string, () => Promise<unknown>> = {
  'lv-filter-panel': () => import('./filters/filter-panel'),
  'lv-filter-card': () => import('./filters/filter-card'),
  'lv-kpi-card': () => import('./charts/echart'),
  'lv-echart': () => import('./charts/echart'),
  'lv-report-table': () => import('./table/report-table'),
}

export function loadDashboardComponent(kind: DashboardComponentKind): Promise<void> {
  const loader = dashboardComponentLoaders[kind]
  if (!loader) return Promise.reject(new Error(`unsupported dashboard component kind ${kind}`))
  return defineElementOnce(kind, loader)
}

export function dashboardComponentKinds(): string[] {
  return Object.keys(dashboardComponentLoaders)
}
