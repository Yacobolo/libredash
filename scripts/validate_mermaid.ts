export type MermaidBlock = {
  file: string
  line: number
  source: string
}

export type MermaidIssue = {
  file: string
  line: number
  message: string
}

export function extractMermaidBlocks(file: string, markdown: string): MermaidBlock[] {
  const lines = markdown.replaceAll('\r\n', '\n').split('\n')
  const blocks: MermaidBlock[] = []

  for (let index = 0; index < lines.length; index += 1) {
    const opening = lines[index].match(/^ {0,3}(`{3,}|~{3,})[ \t]*mermaid(?:[ \t]+.*)?$/i)
    if (!opening) continue

    const marker = opening[1][0]
    const minimumLength = opening[1].length
    const content: string[] = []
    let closing = index + 1
    for (; closing < lines.length; closing += 1) {
      const candidate = lines[closing].match(/^ {0,3}(`+|~+)[ \t]*$/)
      if (candidate && candidate[1][0] === marker && candidate[1].length >= minimumLength) break
      content.push(lines[closing])
    }

    if (closing >= lines.length) {
      blocks.push({ file, line: index + 1, source: content.join('\n') })
      break
    }

    blocks.push({ file, line: index + 1, source: content.join('\n') })
    index = closing
  }

  return blocks
}

export async function validateMermaidBlocks(blocks: MermaidBlock[]): Promise<MermaidIssue[]> {
  const issues: MermaidIssue[] = []
  const mermaid = await loadMermaidParser()
  for (const block of blocks) {
    if (!/^\s*accTitle:\s*\S.+$/m.test(block.source)) {
      issues.push({ file: block.file, line: block.line, message: 'diagram must define accTitle' })
    }
    if (!/^\s*accDescr(?:\s*:\s*\S|\s*\{)/m.test(block.source)) {
      issues.push({ file: block.file, line: block.line, message: 'diagram must define accDescr' })
    }
    try {
      await mermaid.parse(block.source)
    } catch (error) {
      issues.push({
        file: block.file,
        line: block.line,
        message: compactError(error),
      })
    }
  }
  return issues
}

let mermaidParser: Promise<(typeof import('mermaid'))['default']> | undefined

async function loadMermaidParser(): Promise<(typeof import('mermaid'))['default']> {
  if (mermaidParser) return mermaidParser

  mermaidParser = (async () => {
    if (typeof document === 'undefined') {
      const { JSDOM } = await import('jsdom')
      const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://docs.leapview.dev/' })
      const browserGlobals = [
        'window',
        'document',
        'navigator',
        'Node',
        'NodeFilter',
        'Element',
        'HTMLElement',
        'SVGElement',
        'DOMParser',
        'MutationObserver',
        'getComputedStyle',
      ] as const
      const values: Record<(typeof browserGlobals)[number], unknown> = {
        window: dom.window,
        document: dom.window.document,
        navigator: dom.window.navigator,
        Node: dom.window.Node,
        NodeFilter: dom.window.NodeFilter,
        Element: dom.window.Element,
        HTMLElement: dom.window.HTMLElement,
        SVGElement: dom.window.SVGElement,
        DOMParser: dom.window.DOMParser,
        MutationObserver: dom.window.MutationObserver,
        getComputedStyle: dom.window.getComputedStyle.bind(dom.window),
      }
      for (const name of browserGlobals) {
        Object.defineProperty(globalThis, name, { configurable: true, value: values[name] })
      }
    }
    return (await import('mermaid')).default
  })()
  return mermaidParser
}

async function documentationMermaidBlocks(): Promise<MermaidBlock[]> {
  const blocks: MermaidBlock[] = []
  const glob = new Bun.Glob('docs/**/*.md')
  for await (const file of glob.scan({ cwd: '.', onlyFiles: true })) {
    blocks.push(...extractMermaidBlocks(file, await Bun.file(file).text()))
  }
  return blocks
}

function compactError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error)
  return message.replaceAll(/\s+/g, ' ').trim()
}

if (import.meta.main) {
  const blocks = await documentationMermaidBlocks()
  const issues = await validateMermaidBlocks(blocks)
  if (issues.length > 0) {
    for (const issue of issues) console.error(`${issue.file}:${issue.line}: invalid Mermaid diagram: ${issue.message}`)
    process.exit(1)
  }
  console.log(`validated ${blocks.length} Mermaid diagram${blocks.length === 1 ? '' : 's'}`)
}
