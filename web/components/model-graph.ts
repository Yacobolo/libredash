import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import '@xyflow/react/dist/style.css'
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Edge,
  type Node,
} from '@xyflow/react'

type ModelGraphData = {
  name: string
  title: string
  stats: Record<string, number>
  nodes: ModelGraphNode[]
  edges: ModelGraphEdge[]
}

type ModelGraphNode = {
  id: string
  label: string
  kind: string
  schema?: string
  description?: string
  fields?: Array<{ name: string; role?: string }>
  meta?: Array<{ label: string; value: string }>
}

type ModelGraphEdge = {
  id: string
  source: string
  target: string
  label?: string
  kind: string
  cardinality?: string
}

class ModelGraph extends LitElement {
  @property({ attribute: 'data-model' }) dataModel = '{}'
  @state() private selectedID = ''
  private root?: Root
  private mount?: HTMLDivElement

  static styles = css``

  createRenderRoot(): HTMLElement {
    return this
  }

  /*
   * The component renders in light DOM so React Flow's generated stylesheet can
   * style its internal DOM. Component-specific selectors live in app.input.css.
   */
  static unusedShadowStyles = css`
    :host {
      display: block;
      min-height: 0;
      height: 100%;
    }

    .shell {
      display: grid;
      height: 100%;
      min-height: 620px;
      grid-template-columns: minmax(0, 1fr) 280px;
      border: 1px solid var(--borderColor-default);
      border-radius: 8px;
      background: var(--bgColor-default);
      box-shadow: var(--shadow-resting-medium);
      overflow: hidden;
    }

    .flow {
      min-width: 0;
      min-height: 0;
      background:
        linear-gradient(var(--bgColor-default), var(--bgColor-default)),
        radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--fgColor-muted), transparent 86%) 1px, transparent 0);
      background-size: auto, 18px 18px;
    }

    .inspector {
      border-left: 1px solid var(--borderColor-default);
      background: var(--bgColor-muted);
      padding: 12px;
      overflow: auto;
    }

    .inspector h2 {
      margin: 0 0 4px;
      font-size: 0.95rem;
      line-height: 1.15;
    }

    .kind {
      margin: 0 0 12px;
      color: var(--fgColor-muted);
      font-size: 0.68rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .detail {
      display: grid;
      gap: 8px;
    }

    .detail-row {
      display: grid;
      gap: 2px;
      border-bottom: 1px solid var(--borderColor-muted);
      padding-bottom: 7px;
      font-size: 0.74rem;
    }

    .detail-row span:first-child {
      color: var(--fgColor-muted);
      font-size: 0.62rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .fields {
      display: grid;
      gap: 4px;
      margin-top: 10px;
    }

    .field {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      border: 1px solid var(--borderColor-default);
      border-radius: 4px;
      background: var(--bgColor-default);
      padding: 5px 7px;
      font-size: 0.72rem;
      font-weight: 750;
    }

    .field code {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-family: inherit;
    }

    .field span {
      color: var(--fgColor-muted);
      font-size: 0.62rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .empty {
      color: var(--fgColor-muted);
      font-size: 0.78rem;
      line-height: 1.45;
    }

    .node-card {
      width: 214px;
      border: 1px solid var(--node-border);
      border-radius: 6px;
      background: var(--bgColor-default);
      box-shadow: var(--shadow-resting-small);
      color: var(--fgColor-default);
      overflow: hidden;
    }

    .node-card.selected {
      outline: 2px solid var(--fgColor-accent);
      outline-offset: 2px;
    }

    .node-head {
      border-left: 4px solid var(--node-accent);
      background: var(--node-bg);
      padding: 8px 9px 7px;
    }

    .node-kind {
      color: var(--fgColor-muted);
      font-size: 0.58rem;
      font-weight: 950;
      letter-spacing: 0;
      text-transform: uppercase;
    }

    .node-title {
      overflow: hidden;
      margin-top: 2px;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 0.82rem;
      font-weight: 900;
      line-height: 1.15;
    }

    .node-fields {
      display: grid;
      gap: 1px;
      padding: 6px 8px 8px;
    }

    .node-field {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      border-bottom: 1px solid var(--borderColor-muted);
      padding: 3px 0;
      font-size: 0.66rem;
      font-weight: 750;
    }

    .node-field:last-child {
      border-bottom: 0;
    }

    .node-field code {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-family: inherit;
    }

    .node-field span {
      color: var(--fgColor-muted);
      font-size: 0.56rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .react-flow {
      color: var(--fgColor-default);
    }

    .react-flow__attribution {
      display: none;
    }

    .react-flow__controls,
    .react-flow__minimap {
      border: 1px solid var(--borderColor-default);
      background: var(--bgColor-default);
      box-shadow: var(--shadow-resting-small);
    }

    .react-flow__controls-button {
      border-bottom-color: var(--borderColor-muted);
      background: var(--bgColor-default);
      color: var(--fgColor-default);
    }

    @media (max-width: 980px) {
      .shell {
        grid-template-columns: 1fr;
      }

      .inspector {
        max-height: 260px;
        border-top: 1px solid var(--borderColor-default);
        border-left: 0;
      }
    }
  `

  firstUpdated(): void {
    this.mount = this.renderRoot.querySelector('.flow') as HTMLDivElement | null ?? undefined
    if (this.mount) {
      this.root = createRoot(this.mount)
      this.renderFlow()
    }
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('dataModel') || changed.has('selectedID')) {
      this.renderFlow()
    }
  }

  disconnectedCallback(): void {
    this.root?.unmount()
    super.disconnectedCallback()
  }

  render() {
    const graph = this.graph
    const selected = graph.nodes.find((node) => node.id === this.selectedID) ?? graph.nodes[0]
    return html`
      <section class="shell">
        <div class="flow"></div>
        <aside class="inspector" aria-label="Model details">
          ${selected ? this.renderInspector(selected) : html`<p class="empty">Select an entity to inspect fields and relationships.</p>`}
        </aside>
      </section>
    `
  }

  private renderInspector(node: ModelGraphNode) {
    return html`
      <h2>${node.label}</h2>
      <p class="kind">${kindLabel(node.kind)}</p>
      <div class="detail">
        ${node.schema ? html`<div class="detail-row"><span>Schema</span><strong>${node.schema}</strong></div>` : null}
        ${node.description ? html`<div class="detail-row"><span>Description</span><strong>${node.description}</strong></div>` : null}
        ${(node.meta ?? []).map(
          (item) => html`<div class="detail-row"><span>${item.label}</span><strong>${item.value || '-'}</strong></div>`,
        )}
      </div>
      <div class="fields">
        ${(node.fields ?? []).map(
          (field) => html`<div class="field"><code>${field.name}</code><span>${field.role ?? 'field'}</span></div>`,
        )}
      </div>
    `
  }

  private renderFlow(): void {
    if (!this.root) return
    const graph = this.graph
    const nodes = graph.nodes.map((node) => toFlowNode(node, this.selectedID, graph.nodes))
    const edges = graph.edges.map(toFlowEdge)
    this.root.render(
      React.createElement(ReactFlow, {
        nodes,
        edges,
        nodeTypes: { modelNode: ModelNodeComponent },
        fitView: true,
        fitViewOptions: { padding: 0.08 },
        minZoom: 0.5,
        maxZoom: 1.4,
        nodesDraggable: true,
        nodesConnectable: false,
        elementsSelectable: true,
        onNodeClick: (_event: unknown, node: Node) => {
          this.selectedID = String(node.id)
        },
        children: [
          React.createElement(Background, { key: 'background', gap: 18, size: 1 }),
          React.createElement(MiniMap, {
            key: 'minimap',
            pannable: true,
            zoomable: true,
            nodeColor: (node: Node) => nodeColor(String(node.data?.kind ?? 'source')),
            maskColor: 'color-mix(in srgb, var(--bgColor-default), transparent 26%)',
            style: { width: 140, height: 92 },
          }),
          React.createElement(Controls, { key: 'controls', showInteractive: false }),
        ],
      }),
    )
  }

  private get graph(): ModelGraphData {
    try {
      const parsed = JSON.parse(this.dataModel) as ModelGraphData
      return {
        name: parsed.name ?? '',
        title: parsed.title ?? '',
        stats: parsed.stats ?? {},
        nodes: parsed.nodes ?? [],
        edges: parsed.edges ?? [],
      }
    } catch {
      return { name: '', title: '', stats: {}, nodes: [], edges: [] }
    }
  }
}

function toFlowNode(node: ModelGraphNode, selectedID: string, nodes: ModelGraphNode[]): Node {
  const { x, y } = positionFor(node, nodes)
  return {
    id: node.id,
    type: 'modelNode',
    position: { x, y },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: { ...node, selected: node.id === selectedID },
  }
}

function toFlowEdge(edge: ModelGraphEdge): Edge {
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.label || edge.cardinality || '',
    type: edge.kind === 'relationship' ? 'smoothstep' : 'default',
    markerEnd: { type: MarkerType.ArrowClosed },
    style: {
      stroke: edge.kind === 'relationship' ? 'var(--fgColor-accent)' : 'var(--borderColor-accent-emphasis)',
      strokeDasharray: edge.kind === 'semantic' ? '4 4' : undefined,
      strokeWidth: edge.kind === 'relationship' ? 1.8 : 1.4,
    },
    labelStyle: {
      fill: 'var(--fgColor-muted)',
      fontSize: 10,
      fontWeight: 800,
    },
    labelBgStyle: {
      fill: 'var(--bgColor-default)',
      fillOpacity: 0.9,
    },
  }
}

function positionFor(node: ModelGraphNode, nodes: ModelGraphNode[]): { x: number; y: number } {
  const [kind] = node.id.split(':')
  const index = rankWithinKind(node, nodes)
  switch (kind) {
    case 'source':
      return { x: 0, y: index * 116 }
    case 'cache':
      return { x: 300, y: 292 + index * 128 }
    case 'dataset':
      return { x: 560, y: 292 + index * 128 }
    case 'metrics_view':
      return { x: 820, y: index * 128 }
    case 'metric':
      return { x: 1040, y: index * 116 }
    case 'visual':
      return { x: 1060, y: index * 116 }
    case 'table':
      return { x: 1060, y: 620 + index * 116 }
    default:
      return { x: 560, y: index * 116 }
  }
}

function rankWithinKind(node: ModelGraphNode, nodes: ModelGraphNode[]): number {
  const [kind] = node.id.split(':')
  return nodes.filter((candidate) => candidate.id.split(':')[0] === kind).findIndex((candidate) => candidate.id === node.id)
}

function ModelNodeComponent({ data }: { data: ModelGraphNode & { selected?: boolean } }) {
  const kind = data.kind
  const styles = nodeStyle(kind)
  const fields = (data.fields ?? []).slice(0, 7)
  return React.createElement(
    'div',
    { className: `node-card ${data.selected ? 'selected' : ''}`, style: styles },
    React.createElement(Handle, { type: 'target', position: Position.Left }),
    React.createElement(
      'div',
      { className: 'node-head' },
      React.createElement('div', { className: 'node-kind' }, kindLabel(kind)),
      React.createElement('div', { className: 'node-title', title: data.label }, data.label),
    ),
    React.createElement(
      'div',
      { className: 'node-fields' },
      fields.map((field) =>
        React.createElement(
          'div',
          { className: 'node-field', key: `${field.name}:${field.role}` },
          React.createElement('code', null, field.name),
          React.createElement('span', null, field.role ?? 'field'),
        ),
      ),
    ),
    React.createElement(Handle, { type: 'source', position: Position.Right }),
  )
}

function nodeStyle(kind: string): Record<string, string> {
  const palette: Record<string, [string, string, string]> = {
    source: ['var(--data-blue-color-muted)', 'var(--data-blue-color-emphasis)', 'var(--borderColor-default)'],
    cache: ['var(--data-green-color-muted)', 'var(--data-green-color-emphasis)', 'var(--borderColor-success-muted)'],
    dataset: ['var(--data-auburn-color-muted)', 'var(--data-auburn-color-emphasis)', 'var(--borderColor-attention-muted)'],
    metrics_view: ['var(--data-yellow-color-muted)', 'var(--data-yellow-color-emphasis)', 'var(--borderColor-attention-muted)'],
    metric: ['var(--data-yellow-color-muted)', 'var(--data-yellow-color-emphasis)', 'var(--borderColor-attention-muted)'],
    visual: ['var(--data-purple-color-muted)', 'var(--data-purple-color-emphasis)', 'var(--borderColor-accent-muted)'],
    report_table: ['var(--data-coral-color-muted)', 'var(--data-coral-color-emphasis)', 'var(--borderColor-default)'],
  }
  const [bg, accent, border] = palette[kind] ?? palette.source
  return {
    '--node-bg': bg,
    '--node-accent': accent,
    '--node-border': border,
  } as Record<string, string>
}

function nodeColor(kind: string): string {
  switch (kind) {
    case 'cache':
      return 'var(--data-green-color-emphasis)'
    case 'dataset':
      return 'var(--data-auburn-color-emphasis)'
    case 'metrics_view':
      return 'var(--data-yellow-color-emphasis)'
    case 'metric':
      return 'var(--data-yellow-color-emphasis)'
    case 'visual':
      return 'var(--data-purple-color-emphasis)'
    case 'report_table':
      return 'var(--data-coral-color-emphasis)'
    default:
      return 'var(--data-blue-color-emphasis)'
  }
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'source':
      return 'Source table'
    case 'cache':
      return 'DuckDB cache'
    case 'dataset':
      return 'Semantic dataset'
    case 'metrics_view':
      return 'Metrics view'
    case 'metric':
      return 'Metric'
    case 'visual':
      return 'Visual'
    case 'report_table':
      return 'Report table'
    default:
      return kind
  }
}

customElements.define('ld-model-graph', ModelGraph)
