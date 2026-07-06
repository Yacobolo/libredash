export const datastarRuntimeURL = '/static/vendor/datastar-1.0.2.js?v=dev'

export type DatastarEffect = () => void
export type DatastarRuntime = {
  actions: Record<string, unknown>
  effect(fn: () => void): DatastarEffect
  getPath<T = unknown>(path: string): T | undefined
  mergePatch(patch: Record<string, unknown>, options?: Record<string, unknown>): void
  mergePaths(paths: [string, unknown][], options?: Record<string, unknown>): void
  root: Record<string, unknown>
}

let runtimePromise: Promise<DatastarRuntime> | null = null

export function loadDatastarRuntime(): Promise<DatastarRuntime> {
  runtimePromise ??= import(datastarRuntimeURL) as Promise<DatastarRuntime>
  return runtimePromise
}
