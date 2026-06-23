import { LitElement, html } from 'lit'
import { property } from 'lit/decorators.js'
import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import '@xyflow/react/dist/style.css'
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
} from '@xyflow/react'

type LineageGraph = {
  nodes: LineageNode[]
  edges: LineageEdge[]
}

type LineageNode = {
  id: string
  label: string
  kind: string
  meta?: string
  href?: string
  side?: 'upstream' | 'selected' | 'downstream'
  selected?: boolean
}

type LineageEdge = {
  id: string
  source: string
  target: string
  label?: string
  kind: string
}

class AssetLineageGraph extends LitElement {
  @property({ type: Object }) graph: LineageGraph | null = null
  private root?: Root
  private mount?: HTMLDivElement

  createRenderRoot(): HTMLElement {
    return this
  }

  firstUpdated(): void {
    this.mount = this.renderRoot.querySelector('.asset-lineage-flow') as HTMLDivElement | null ?? undefined
    if (this.mount) {
      this.root = createRoot(this.mount)
      this.renderFlow()
    }
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('graph')) this.renderFlow()
  }

  disconnectedCallback(): void {
    this.root?.unmount()
    super.disconnectedCallback()
  }

  render() {
    return html`
      <style>
        ${assetLineageGraphStyles}
      </style>
      <div class="asset-lineage-flow" aria-label="Asset lineage graph"></div>
    `
  }

  private renderFlow(): void {
    if (!this.root) return
    const graph = this.resolvedGraph
    this.root.render(
      React.createElement(ReactFlow, {
        nodes: graph.nodes.map((node) => toFlowNode(node, graph.nodes)),
        edges: graph.edges.map(toFlowEdge),
        nodeTypes: { lineageNode: LineageNodeComponent },
        fitView: true,
        fitViewOptions: { padding: 0.12 },
        minZoom: 0.15,
        maxZoom: 1.35,
        nodesDraggable: false,
        nodesConnectable: false,
        elementsSelectable: false,
        panOnDrag: true,
        zoomOnScroll: false,
        preventScrolling: false,
        children: [
          React.createElement(Background, { key: 'background', gap: 18, size: 1 }),
          React.createElement(Controls, { key: 'controls', showInteractive: false }),
        ],
      }),
    )
  }

  private get resolvedGraph(): LineageGraph {
    if (this.graph) {
      return {
        nodes: this.graph.nodes ?? [],
        edges: this.graph.edges ?? [],
      }
    }
    return { nodes: [], edges: [] }
  }
}

const assetLineageGraphStyles = `
  ld-asset-lineage-graph .asset-lineage-flow {
    height: 100%;
    min-height: 0;
    min-width: 0;
    background:
      linear-gradient(var(--ld-bg-page), var(--ld-bg-page)),
      radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--ld-fg-muted), transparent 87%) 1px, transparent 0);
    background-size: auto, 18px 18px;
  }

  ld-asset-lineage-graph .react-flow {
    color: var(--ld-fg-default);
  }

  ld-asset-lineage-graph .react-flow__attribution {
    display: none;
  }

  ld-asset-lineage-graph .react-flow__controls {
    border: var(--ld-border-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small);
  }

  ld-asset-lineage-graph .react-flow__controls-button {
    border-bottom-color: var(--ld-line-muted);
    background: var(--ld-bg-panel);
    color: var(--ld-fg-default);
  }

  ld-asset-lineage-graph .asset-lineage-node {
    width: 200px;
    border: var(--borderWidth-default) solid var(--lineage-node-border);
    border-left: var(--borderWidth-thicker) solid var(--lineage-node-accent);
    border-radius: var(--borderRadius-default);
    background: var(--lineage-node-bg);
    box-shadow: var(--shadow-resting-small);
    color: var(--ld-fg-default);
    padding: var(--base-size-8) var(--base-size-12);
  }

  ld-asset-lineage-graph .asset-lineage-node-selected {
    border-color: var(--ld-line-accent);
    box-shadow: 0 0 0 var(--borderWidth-default) color-mix(in srgb, var(--ld-line-accent), transparent 28%), var(--shadow-resting-small);
  }

  ld-asset-lineage-graph .asset-lineage-node-kind {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  ld-asset-lineage-graph .asset-lineage-node-title {
    display: block;
    overflow: hidden;
    margin-top: var(--base-size-4);
    color: var(--ld-fg-default);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-strong);
    line-height: var(--ld-line-height-tight);
    text-decoration: none;
  }

  ld-asset-lineage-graph .asset-lineage-node-title[href]:hover,
  ld-asset-lineage-graph .asset-lineage-node-title[href]:focus-visible {
    color: var(--ld-fg-link);
    outline: 0;
    text-decoration: underline;
  }

  ld-asset-lineage-graph .asset-lineage-node-meta {
    overflow: hidden;
    margin-top: var(--base-size-6);
    color: var(--ld-fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
  }
`

function toFlowNode(node: LineageNode, nodes: LineageNode[]): Node {
  const { x, y } = positionFor(node, nodes)
  return {
    id: node.id,
    type: 'lineageNode',
    position: { x, y },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: node,
  }
}

function toFlowEdge(edge: LineageEdge): Edge {
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.label ?? '',
    markerEnd: { type: MarkerType.ArrowClosed },
    style: {
      stroke: edgeStroke(edge.kind),
      strokeWidth: 1.5,
    },
    labelStyle: {
      fill: 'var(--ld-fg-muted)',
      fontSize: 10,
      fontWeight: 500,
    },
    labelBgStyle: {
      fill: 'var(--ld-bg-page)',
      fillOpacity: 0.92,
    },
  }
}

function positionFor(node: LineageNode, nodes: LineageNode[]): { x: number; y: number } {
  if (isDashboardDataLineage(nodes)) return dashboardDataPositionFor(node, nodes)

  const side = node.side ?? 'downstream'
  const sideNodes = nodes.filter((candidate) => (candidate.side ?? 'downstream') === side)
  const index = Math.max(0, sideNodes.findIndex((candidate) => candidate.id === node.id))
  const y = Math.max(48, index * 118 + 48)
  switch (side) {
    case 'upstream':
      return { x: 96, y }
    case 'selected':
      return { x: 384, y: Math.max(96, y) }
    default:
      return { x: 672, y }
  }
}

function isDashboardDataLineage(nodes: LineageNode[]): boolean {
  return nodes.some((node) => node.selected && node.kind === 'dashboard')
}

function dashboardDataPositionFor(node: LineageNode, nodes: LineageNode[]): { x: number; y: number } {
  if (node.selected) return { x: 1440, y: 300 }

  const xByKind: Record<string, number> = {
    connection: 96,
    source: 336,
    model_table: 576,
    semantic_model: 816,
    measure: 1200,
  }
  const x = xByKind[node.kind] ?? 576
  if (node.kind !== 'source') return { x, y: 300 }

  const sources = nodes.filter((candidate) => candidate.kind === 'source').sort((left, right) => left.label.localeCompare(right.label))
  const index = Math.max(0, sources.findIndex((candidate) => candidate.id === node.id))
  const y = 60 + index * 92
  return { x, y }
}

function LineageNodeComponent({ data }: { data: LineageNode }) {
  const styles = nodeStyle(data)
  const className = data.selected ? 'asset-lineage-node asset-lineage-node-selected' : 'asset-lineage-node'
  return React.createElement(
    'div',
    { className, style: styles },
    React.createElement(Handle, { type: 'target', position: Position.Left }),
    React.createElement('div', { className: 'asset-lineage-node-kind' }, kindLabel(data.kind)),
    data.href
      ? React.createElement('a', { className: 'asset-lineage-node-title', href: data.href, title: data.label }, data.label)
      : React.createElement('div', { className: 'asset-lineage-node-title', title: data.label }, data.label),
    data.meta ? React.createElement('div', { className: 'asset-lineage-node-meta' }, data.meta) : null,
    React.createElement(Handle, { type: 'source', position: Position.Right }),
  )
}

function nodeStyle(node: LineageNode): Record<string, string> {
  const palette: Record<string, [string, string, string]> = {
    catalog: ['var(--ld-asset-catalog-bg)', 'var(--ld-asset-catalog-accent)', 'var(--ld-asset-catalog-border)'],
    connection: ['var(--ld-asset-connection-bg)', 'var(--ld-asset-connection-accent)', 'var(--ld-asset-connection-border)'],
    dashboard: ['var(--ld-asset-dashboard-bg)', 'var(--ld-asset-dashboard-accent)', 'var(--ld-asset-dashboard-border)'],
    dimension: ['var(--ld-asset-dimension-bg)', 'var(--ld-asset-dimension-accent)', 'var(--ld-asset-dimension-border)'],
    filter: ['var(--ld-asset-filter-bg)', 'var(--ld-asset-filter-accent)', 'var(--ld-asset-filter-border)'],
    measure: ['var(--ld-asset-measure-bg)', 'var(--ld-asset-measure-accent)', 'var(--ld-asset-measure-border)'],
    model_table: ['var(--ld-asset-model-table-bg)', 'var(--ld-asset-model-table-accent)', 'var(--ld-asset-model-table-border)'],
    page: ['var(--ld-asset-page-bg)', 'var(--ld-asset-page-accent)', 'var(--ld-asset-page-border)'],
    semantic_model: ['var(--ld-asset-semantic-model-bg)', 'var(--ld-asset-semantic-model-accent)', 'var(--ld-asset-semantic-model-border)'],
    source: ['var(--ld-asset-source-bg)', 'var(--ld-asset-source-accent)', 'var(--ld-asset-source-border)'],
    table: ['var(--ld-asset-table-bg)', 'var(--ld-asset-table-accent)', 'var(--ld-asset-table-border)'],
    visual: ['var(--ld-asset-visual-bg)', 'var(--ld-asset-visual-accent)', 'var(--ld-asset-visual-border)'],
    // Compatibility-only asset kinds share the model-table palette so old
    // deployment/API data does not reintroduce separate product concepts.
    cache_table: ['var(--ld-asset-model-table-bg)', 'var(--ld-asset-model-table-accent)', 'var(--ld-asset-model-table-border)'],
    dataset: ['var(--ld-asset-model-table-bg)', 'var(--ld-asset-model-table-accent)', 'var(--ld-asset-model-table-border)'],
  }
  const [bg, accent, border] = palette[node.kind] ?? palette.semantic_model
  return {
    '--lineage-node-bg': bg,
    '--lineage-node-accent': node.selected ? 'var(--ld-line-accent)' : accent,
    '--lineage-node-border': border,
  } as Record<string, string>
}

function edgeStroke(kind: string): string {
  if (kind.startsWith('uses')) return 'var(--ld-line-accent)'
  if (kind.startsWith('reads')) return 'var(--ld-fg-warning)'
  if (kind.startsWith('filters')) return 'var(--ld-fg-success)'
  return 'var(--ld-line-muted)'
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'cache_table':
      return 'Materialization'
    case 'dataset':
      return 'Model table'
    case 'model_table':
      return 'Model table'
    case 'semantic_model':
      return 'Semantic model'
    default:
      return kind.replaceAll('_', ' ').replace(/\b\w/g, (char) => char.toUpperCase())
  }
}

customElements.define('ld-asset-lineage-graph', AssetLineageGraph)
