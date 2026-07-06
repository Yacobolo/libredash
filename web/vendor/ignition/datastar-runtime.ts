export type Effect = () => void
export type JSONPatch = Record<string, unknown>
export type MergePatchArgs = { ifMissing?: boolean }
export type Paths = [string, unknown][]

import { loadDatastarRuntime } from '../../components/shared/datastar-runtime'

const runtime = await loadDatastarRuntime()

export const actions = runtime.actions
export const effect = runtime.effect
export const mergePatch = runtime.mergePatch
export const mergePaths = runtime.mergePaths
export const root = runtime.root
