import { LitElement, html } from 'lit'
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

  createRenderRoot(): HTMLElement {
    return this
  }

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
      <style>
        ${modelGraphStyles}
      </style>
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
            maskColor: 'color-mix(in srgb, var(--ld-bg-page), transparent 26%)',
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

const modelGraphStyles = `
  ld-model-graph {
    display: block;
    min-height: 0;
    height: 100%;
    width: 100%;
  }

  ld-model-graph .shell {
    display: grid;
    height: 100%;
    min-height: 620px;
    grid-template-columns: minmax(0, 1fr) 280px;
    border: var(--ld-border-default);
    border-radius: var(--borderRadius-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-medium);
    overflow: hidden;
  }

  ld-model-graph .flow {
    min-width: 0;
    min-height: 0;
    background:
      linear-gradient(var(--ld-bg-page), var(--ld-bg-page)),
      radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--ld-fg-muted), transparent 86%) 1px, transparent 0);
    background-size: auto, 18px 18px;
  }

  ld-model-graph .inspector {
    border-left: var(--ld-border-default);
    background: var(--ld-bg-panel-muted);
    padding: 12px;
    overflow: auto;
  }

  ld-model-graph .inspector h2 {
    margin: 0 0 4px;
    font-size: var(--ld-font-size-body-lg);
    line-height: var(--ld-line-height-tight);
  }

  ld-model-graph .kind {
    margin: 0 0 12px;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  ld-model-graph .detail {
    display: grid;
    gap: 8px;
  }

  ld-model-graph .detail-row {
    display: grid;
    gap: 2px;
    border-bottom: var(--ld-border-muted);
    padding-bottom: 7px;
    font-size: var(--ld-font-size-caption);
  }

  ld-model-graph .detail-row span:first-child {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  ld-model-graph .fields {
    display: grid;
    gap: 4px;
    margin-top: 10px;
  }

  ld-model-graph .field {
    display: flex;
    min-width: 0;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    border: var(--ld-border-default);
    border-radius: var(--borderRadius-small);
    background: var(--ld-bg-panel);
    padding: 5px 7px;
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
  }

  ld-model-graph .field code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: inherit;
  }

  ld-model-graph .field span {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  ld-model-graph .empty {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-body-md);
    line-height: var(--ld-line-height-normal);
  }

  ld-model-graph .node-card {
    width: 214px;
    border: 1px solid var(--node-border);
    border-radius: var(--borderRadius-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small);
    color: var(--ld-fg-default);
    overflow: hidden;
  }

  ld-model-graph .node-card.selected {
    outline: var(--ld-border-width-focus) solid var(--ld-fg-link);
    outline-offset: 2px;
  }

  ld-model-graph .node-head {
    border-left: 4px solid var(--node-accent);
    background: var(--node-bg);
    padding: 8px 9px 7px;
  }

  ld-model-graph .node-kind {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    letter-spacing: 0;
    text-transform: uppercase;
  }

  ld-model-graph .node-title {
    overflow: hidden;
    margin-top: 2px;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-strong);
    line-height: var(--ld-line-height-tight);
  }

  ld-model-graph .node-fields {
    display: grid;
    gap: 1px;
    padding: 6px 8px 8px;
  }

  ld-model-graph .node-field {
    display: flex;
    min-width: 0;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    border-bottom: var(--ld-border-muted);
    padding: 3px 0;
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
  }

  ld-model-graph .node-field:last-child {
    border-bottom: 0;
  }

  ld-model-graph .node-field code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: inherit;
  }

  ld-model-graph .node-field span {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  ld-model-graph .react-flow {
    color: var(--ld-fg-default);
  }

  ld-model-graph .react-flow__attribution {
    display: none;
  }

  ld-model-graph .react-flow__controls,
  ld-model-graph .react-flow__minimap {
    border: var(--ld-border-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small);
  }

  ld-model-graph .react-flow__controls-button {
    border-bottom-color: var(--ld-line-muted);
    background: var(--ld-bg-panel);
    color: var(--ld-fg-default);
  }

  @media (max-width: 980px) {
    ld-model-graph .shell {
      grid-template-columns: 1fr;
    }

    ld-model-graph .inspector {
      max-height: 260px;
      border-top: var(--ld-border-default);
      border-left: 0;
    }
  }
`

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
      stroke: edge.kind === 'relationship' ? 'var(--ld-fg-link)' : 'var(--ld-line-accent)',
      strokeDasharray: edge.kind === 'semantic' ? '4 4' : undefined,
      strokeWidth: edge.kind === 'relationship' ? 1.8 : 1.4,
    },
    labelStyle: {
      fill: 'var(--ld-fg-muted)',
      fontSize: 10,
      fontWeight: 500,
    },
    labelBgStyle: {
      fill: 'var(--ld-bg-page)',
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
    source: ['var(--ld-asset-source-bg)', 'var(--ld-asset-source-accent)', 'var(--ld-asset-source-border)'],
    cache: ['var(--ld-asset-cache-table-bg)', 'var(--ld-asset-cache-table-accent)', 'var(--ld-asset-cache-table-border)'],
    dataset: ['var(--ld-asset-dataset-bg)', 'var(--ld-asset-dataset-accent)', 'var(--ld-asset-dataset-border)'],
    metrics_view: ['var(--ld-asset-metric-view-bg)', 'var(--ld-asset-metric-view-accent)', 'var(--ld-asset-metric-view-border)'],
    metric: ['var(--ld-asset-measure-bg)', 'var(--ld-asset-measure-accent)', 'var(--ld-asset-measure-border)'],
    visual: ['var(--ld-asset-visual-bg)', 'var(--ld-asset-visual-accent)', 'var(--ld-asset-visual-border)'],
    report_table: ['var(--ld-asset-table-bg)', 'var(--ld-asset-table-accent)', 'var(--ld-asset-table-border)'],
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
      return 'var(--ld-asset-cache-table-accent)'
    case 'dataset':
      return 'var(--ld-asset-dataset-accent)'
    case 'metrics_view':
      return 'var(--ld-asset-metric-view-accent)'
    case 'metric':
      return 'var(--ld-asset-measure-accent)'
    case 'visual':
      return 'var(--ld-asset-visual-accent)'
    case 'report_table':
      return 'var(--ld-asset-table-accent)'
    default:
      return 'var(--ld-asset-source-accent)'
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
