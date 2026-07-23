import { View, changeset, parse } from 'vega'
import { compile } from 'vega-lite'
import { expressionInterpreter } from 'vega-interpreter'

let port: MessagePort | undefined
let view: View | undefined

window.addEventListener('message', (event) => {
  if (event.data?.kind !== 'connect' || event.ports.length !== 1 || port) return
  port = event.ports[0]
  port.onmessage = (message) => void receive(message.data)
})

async function receive(message: any): Promise<void> {
  try {
    if (message.kind === 'render') await render(message.program, message.datasets)
    if (message.kind === 'resize' && view) await view.width(message.width).height(message.height).runAsync()
    if (message.kind === 'snapshot' && view) {
      const canvas = await view.toCanvas(2)
      const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((value) => value ? resolve(value) : reject(new Error('snapshot failed')), 'image/png'))
      port?.postMessage({ kind: 'snapshot', id: message.id, blob })
    }
  } catch (error) {
    port?.postMessage({ kind: message.kind === 'snapshot' ? 'snapshot' : 'error', id: message.id, error: error instanceof Error ? error.message : String(error) })
  }
}

async function render(program: object, datasets: Record<string, Record<string, unknown>[]>): Promise<void> {
  view?.finalize()
  const compiled = compile(program as any).spec
  const runtime = parse(compiled as any, undefined, { ast: true })
  view = new View(runtime, { renderer: 'canvas', container: '#view', hover: true, expr: expressionInterpreter })
  view.addEventListener('click', (_event, item) => {
    const datum = item?.datum
    if (!datum || typeof datum !== 'object') return
    const scalar = Object.fromEntries(Object.entries(datum).filter(([, value]) => value === null || ['string', 'number', 'boolean'].includes(typeof value)))
    port?.postMessage({ kind: 'interaction', datum: scalar })
  })
  for (const [name, rows] of Object.entries(datasets)) view.change(name, changeset().remove(() => true).insert(rows))
  await view.runAsync()
}
