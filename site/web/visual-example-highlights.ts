type YAMLNode = {
  container: boolean
  indent: number
  line: number
  path: string
}

const yamlMappingKey = /^(\s*)(?:-\s+)?([A-Za-z0-9_-]+):(?:\s|$)/

export function visualExampleHighlightLines(source: string, fieldPaths: readonly string[]): number[] {
  const wanted = new Set(fieldPaths)
  if (wanted.size === 0) return []

  const lines = source.split('\n')
  const stack: YAMLNode[] = []
  const nodes: YAMLNode[] = []

  lines.forEach((line, index) => {
    const match = line.match(yamlMappingKey)
    if (!match) return

    const indent = match[1].length
    while (stack.at(-1) && stack.at(-1)!.indent >= indent) stack.pop()

    const node: YAMLNode = {
      container: line.trimEnd().endsWith(':'),
      indent,
      line: index + 1,
      path: [...stack.map((ancestor) => ancestor.path.split('.').at(-1)!), match[2]].join('.'),
    }
    nodes.push(node)
    stack.push(node)
  })

  const highlighted = new Set<number>()
  for (const node of nodes) {
    const relativePath = node.path.split('.').slice(2).join('.')
    if (!wanted.has(relativePath)) continue

    highlighted.add(node.line)
    if (!node.container) continue

    for (let index = node.line; index < lines.length; index += 1) {
      const descendant = lines[index]
      if (!descendant.trim()) continue
      const indent = descendant.match(/^\s*/)?.[0].length ?? 0
      if (indent <= node.indent) break
      highlighted.add(index + 1)
    }
  }

  return [...highlighted].sort((left, right) => left - right)
}
