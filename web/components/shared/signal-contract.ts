type ContractShape = Record<string, 'required' | 'optional'>

export function checkSignalContract(name: string, value: unknown, shape: ContractShape): void {
  if (!isDev()) return
  const object = value && typeof value === 'object' ? value as Record<string, unknown> : {}
  const missing = Object.entries(shape)
    .filter(([, required]) => required === 'required')
    .map(([key]) => key)
    .filter((key) => object[key] === undefined)
  if (missing.length === 0) return
  console.warn(`[LibreDash] ${name} is missing signal fields: ${missing.join(', ')}`)
}

function isDev(): boolean {
  return location.hostname === 'localhost' || location.hostname === '127.0.0.1' || location.search.includes('debugSignals=1')
}
