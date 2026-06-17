import { LitElement, css, html } from 'lit'
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

type UsageGraph = {
  nodes: UsageNode[]
  edges: UsageEdge[]
}

type UsageNode = {
  id: string
  label: string
  kind: string
  meta?: string
}

type UsageEdge = {
  id: string
  source: string
  target: string
  label?: string
  kind: string
}

class MetricUsageGraph extends LitElement {
  @property({ type: Object }) graph: UsageGraph | null = null
  @property({ attribute: 'data-graph' }) dataGraph = '{}'
  private root?: Root
  private mount?: HTMLDivElement

  static styles = css``

  createRenderRoot(): HTMLElement {
    return this
  }

  firstUpdated(): void {
    this.mount = this.renderRoot.querySelector('.metric-usage-flow') as HTMLDivElement | null ?? undefined
    if (this.mount) {
      this.root = createRoot(this.mount)
      this.renderFlow()
    }
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('graph') || changed.has('dataGraph')) this.renderFlow()
  }

  disconnectedCallback(): void {
    this.root?.unmount()
    super.disconnectedCallback()
  }

  render() {
    return html`<div class="metric-usage-flow" aria-label="Metric usage lineage"></div>`
  }

  private renderFlow(): void {
    if (!this.root) return
    const graph = this.resolvedGraph
    this.root.render(
      React.createElement(ReactFlow, {
        nodes: graph.nodes.map((node) => toFlowNode(node, graph.nodes)),
        edges: graph.edges.map(toFlowEdge),
        nodeTypes: { usageNode: UsageNodeComponent },
        fitView: true,
        fitViewOptions: { padding: 0.18 },
        minZoom: 0.55,
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

  private get resolvedGraph(): UsageGraph {
    if (this.graph) {
      return {
        nodes: this.graph.nodes ?? [],
        edges: this.graph.edges ?? [],
      }
    }
    try {
      const parsed = JSON.parse(this.dataGraph) as UsageGraph
      return {
        nodes: parsed.nodes ?? [],
        edges: parsed.edges ?? [],
      }
    } catch {
      return { nodes: [], edges: [] }
    }
  }
}

function toFlowNode(node: UsageNode, nodes: UsageNode[]): Node {
  const { x, y } = positionFor(node, nodes)
  return {
    id: node.id,
    type: 'usageNode',
    position: { x, y },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: node,
  }
}

function toFlowEdge(edge: UsageEdge): Edge {
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.label ?? '',
    markerEnd: { type: MarkerType.ArrowClosed },
    style: {
      stroke: edge.kind === 'usage' ? 'var(--fgColor-attention)' : 'var(--borderColor-accent-emphasis)',
      strokeWidth: edge.kind === 'usage' ? 1.8 : 1.4,
    },
    labelStyle: {
      fill: 'var(--fgColor-muted)',
      fontSize: 10,
      fontWeight: 800,
    },
    labelBgStyle: {
      fill: 'var(--bgColor-default)',
      fillOpacity: 0.92,
    },
  }
}

function positionFor(node: UsageNode, nodes: UsageNode[]): { x: number; y: number } {
  const index = nodes.filter((candidate) => candidate.kind === node.kind).findIndex((candidate) => candidate.id === node.id)
  switch (node.kind) {
    case 'model':
      return { x: 0, y: 92 }
    case 'dataset':
      return { x: 250, y: 92 }
    case 'metrics_view':
      return { x: 500, y: 92 }
    case 'dashboard':
      return { x: 760, y: Math.max(18, index * 118) }
    default:
      return { x: 250, y: index * 118 }
  }
}

function UsageNodeComponent({ data }: { data: UsageNode }) {
  const styles = nodeStyle(data.kind)
  return React.createElement(
    'div',
    { className: 'metric-usage-node', style: styles },
    React.createElement(Handle, { type: 'target', position: Position.Left }),
    React.createElement('div', { className: 'metric-usage-node-kind' }, kindLabel(data.kind)),
    React.createElement('div', { className: 'metric-usage-node-title', title: data.label }, data.label),
    data.meta ? React.createElement('div', { className: 'metric-usage-node-meta' }, data.meta) : null,
    React.createElement(Handle, { type: 'source', position: Position.Right }),
  )
}

function nodeStyle(kind: string): Record<string, string> {
  const palette: Record<string, [string, string, string]> = {
    model: ['var(--data-blue-color-muted)', 'var(--data-blue-color-emphasis)', 'var(--borderColor-accent-muted)'],
    dataset: ['var(--data-auburn-color-muted)', 'var(--data-auburn-color-emphasis)', 'var(--borderColor-attention-muted)'],
    metrics_view: ['var(--data-yellow-color-muted)', 'var(--data-yellow-color-emphasis)', 'var(--borderColor-attention-muted)'],
    dashboard: ['var(--data-purple-color-muted)', 'var(--data-purple-color-emphasis)', 'var(--borderColor-accent-muted)'],
  }
  const [bg, accent, border] = palette[kind] ?? palette.model
  return {
    '--usage-node-bg': bg,
    '--usage-node-accent': accent,
    '--usage-node-border': border,
  } as Record<string, string>
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'model':
      return 'Semantic model'
    case 'dataset':
      return 'Dataset'
    case 'metrics_view':
      return 'Metrics view'
    case 'dashboard':
      return 'Dashboard'
    default:
      return kind
  }
}

customElements.define('ld-metric-usage-graph', MetricUsageGraph)
