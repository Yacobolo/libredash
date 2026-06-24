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
  rank?: number
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
    const layout = lineageLayout(graph)
    this.root.render(
      React.createElement(ReactFlow, {
        nodes: graph.nodes.map((node) => toFlowNode(node, layout)),
        edges: graph.edges.map((edge) => toFlowEdge(edge, layout)),
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

type LineageLayout = {
  nodes: LineageNode[]
  ranks: Map<string, number>
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

function lineageLayout(graph: LineageGraph): LineageLayout {
  return {
    nodes: graph.nodes,
    ranks: visualRanks(graph.nodes, graph.edges),
  }
}

function toFlowNode(node: LineageNode, layout: LineageLayout): Node {
  const { x, y } = positionFor(node, layout)
  return {
    id: node.id,
    type: 'lineageNode',
    position: { x, y },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: node,
  }
}

function toFlowEdge(edge: LineageEdge, layout: LineageLayout): Edge {
  const context = edge.kind === 'contains'
  const { source, target } = visualEdgeEndpoints(edge, layout)
  return {
    id: edge.id,
    source,
    target,
    label: context ? '' : edge.label ?? '',
    type: context ? 'smoothstep' : 'default',
    markerEnd: context ? undefined : { type: MarkerType.ArrowClosed },
    interactionWidth: context ? 8 : 14,
    style: {
      stroke: edgeStroke(edge.kind),
      strokeWidth: context ? 1 : 1.8,
      strokeDasharray: context ? '4 7' : undefined,
      opacity: context ? 0.28 : 0.9,
    },
    labelStyle: {
      fill: context ? 'color-mix(in srgb, var(--ld-fg-muted), transparent 12%)' : 'var(--ld-fg-muted)',
      fontSize: 10,
      fontWeight: 500,
    },
    labelBgStyle: {
      fill: 'var(--ld-bg-page)',
      fillOpacity: 0.92,
    },
  }
}

function visualRanks(nodes: LineageNode[], edges: LineageEdge[]): Map<string, number> {
  const ranks = new Map(nodes.map((node) => [node.id, nodeRank(node)]))
  const layout: LineageLayout = { nodes, ranks }
  for (let pass = 0; pass < nodes.length; pass += 1) {
    let changed = false
    for (const edge of edges) {
      const { source, target } = visualEdgeEndpoints(edge, layout)
      const sourceRank = ranks.get(source) ?? 0
      const targetRank = ranks.get(target) ?? 0
      if (targetRank <= sourceRank) {
        ranks.set(target, sourceRank + 1)
        changed = true
      }
    }
    if (!changed) break
  }
  return ranks
}

function visualEdgeEndpoints(edge: LineageEdge, layout: LineageLayout): { source: string; target: string } {
  const byID = new Map(layout.nodes.map((node) => [node.id, node]))
  const source = byID.get(edge.source)
  const target = byID.get(edge.target)
  if (!source || !target) return { source: edge.source, target: edge.target }
  const sourceRank = layout.ranks.get(source.id) ?? nodeRank(source)
  const targetRank = layout.ranks.get(target.id) ?? nodeRank(target)
  if (sourceRank > targetRank || (sourceRank === targetRank && nodeSortKey(source) > nodeSortKey(target))) {
    return { source: edge.target, target: edge.source }
  }
  return { source: edge.source, target: edge.target }
}

function positionFor(node: LineageNode, layout: LineageLayout): { x: number; y: number } {
  const ranks = Array.from(new Set(layout.nodes.map((candidate) => visualRank(candidate, layout)))).sort((left, right) => left - right)
  const rank = visualRank(node, layout)
  const rankIndex = Math.max(0, ranks.indexOf(rank))
  const rankNodes = layout.nodes
    .filter((candidate) => visualRank(candidate, layout) === rank)
    .sort((left, right) => nodeSortKey(left).localeCompare(nodeSortKey(right)))
  const index = Math.max(0, rankNodes.findIndex((candidate) => candidate.id === node.id))
  const centeredOffset = (index - (rankNodes.length - 1) / 2) * 124
  return {
    x: 96 + rankIndex * 260,
    y: Math.max(48, 240 + centeredOffset),
  }
}

function visualRank(node: LineageNode, layout: LineageLayout): number {
  return layout.ranks.get(node.id) ?? nodeRank(node)
}

function nodeRank(node: LineageNode): number {
  if (typeof node.rank === 'number' && Number.isFinite(node.rank)) return node.rank
  if (node.selected || node.side === 'selected') return 0
  if (node.side === 'upstream') return -1
  return 1
}

function nodeSortKey(node: LineageNode): string {
  return `${node.kind}:${node.label}:${node.id}`
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
    field: ['var(--ld-asset-dimension-bg)', 'var(--ld-asset-dimension-accent)', 'var(--ld-asset-dimension-border)'],
    filter: ['var(--ld-asset-filter-bg)', 'var(--ld-asset-filter-accent)', 'var(--ld-asset-filter-border)'],
    measure: ['var(--ld-asset-measure-bg)', 'var(--ld-asset-measure-accent)', 'var(--ld-asset-measure-border)'],
    model_table: ['var(--ld-asset-model-table-bg)', 'var(--ld-asset-model-table-accent)', 'var(--ld-asset-model-table-border)'],
    page: ['var(--ld-asset-page-bg)', 'var(--ld-asset-page-accent)', 'var(--ld-asset-page-border)'],
    page_item: ['var(--ld-asset-page-bg)', 'var(--ld-asset-page-accent)', 'var(--ld-asset-page-border)'],
    relationship: ['var(--ld-asset-dimension-bg)', 'var(--ld-asset-dimension-accent)', 'var(--ld-asset-dimension-border)'],
    semantic_model: ['var(--ld-asset-semantic-model-bg)', 'var(--ld-asset-semantic-model-accent)', 'var(--ld-asset-semantic-model-border)'],
    semantic_table: ['var(--ld-asset-model-table-bg)', 'var(--ld-asset-model-table-accent)', 'var(--ld-asset-model-table-border)'],
    source: ['var(--ld-asset-source-bg)', 'var(--ld-asset-source-accent)', 'var(--ld-asset-source-border)'],
    table: ['var(--ld-asset-table-bg)', 'var(--ld-asset-table-accent)', 'var(--ld-asset-table-border)'],
    visual: ['var(--ld-asset-visual-bg)', 'var(--ld-asset-visual-accent)', 'var(--ld-asset-visual-border)'],
  }
  const [bg, accent, border] = palette[node.kind] ?? palette.semantic_model
  return {
    '--lineage-node-bg': bg,
    '--lineage-node-accent': node.selected ? 'var(--ld-line-accent)' : accent,
    '--lineage-node-border': border,
  } as Record<string, string>
}

function edgeStroke(kind: string): string {
  if (kind === 'contains') return 'var(--ld-line-muted)'
  if (kind.startsWith('uses')) return 'var(--ld-line-accent)'
  if (kind.startsWith('reads')) return 'var(--ld-fg-warning)'
  if (kind.startsWith('filters')) return 'var(--ld-fg-success)'
  return 'var(--ld-line-muted)'
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'model_table':
      return 'Model table'
    case 'page_item':
      return 'Page item'
    case 'semantic_model':
      return 'Semantic model'
    case 'semantic_table':
      return 'Semantic table'
    default:
      return kind.replaceAll('_', ' ').replace(/\b\w/g, (char) => char.toUpperCase())
  }
}

customElements.define('ld-asset-lineage-graph', AssetLineageGraph)
