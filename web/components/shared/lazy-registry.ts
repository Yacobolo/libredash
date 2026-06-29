type ElementLoader = () => Promise<unknown>

const loading = new Map<string, Promise<void>>()

export function defineElementOnce(name: string, loader: ElementLoader): Promise<void> {
  const existing = customElements.get(name)
  if (existing) return Promise.resolve()

  const pending = loading.get(name)
  if (pending) return pending

  const next = loader().then(() => {
    if (!customElements.get(name)) {
      throw new Error(`custom element ${name} was not registered by its loader`)
    }
  }).finally(() => {
    loading.delete(name)
  })
  loading.set(name, next)
  return next
}

export function ensureElementForKind(kind: string, registry: Record<string, ElementLoader>): Promise<void> {
  const loader = registry[kind]
  if (!loader) return Promise.reject(new Error(`unsupported component kind ${kind}`))
  return defineElementOnce(kind, loader)
}
