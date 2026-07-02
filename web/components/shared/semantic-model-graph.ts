import { LitElement, html } from 'lit'
import { property } from 'lit/decorators.js'
import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { Binary, Braces, CalendarDays, CircleHelp, Clock3, Hash, RotateCcw, Table2, ToggleLeft, Type, type IconNode } from 'lucide'
import '@xyflow/react/dist/style.css'
import {
  Background,
  Controls,
  EdgeLabelRenderer,
  getBezierPath,
  Handle,
  Panel,
  Position,
  ReactFlow,
  type Edge,
  type EdgeProps,
  type Node,
  useNodesState,
} from '@xyflow/react'
import type {
  SemanticModelGraphEdgeSignal,
  SemanticModelGraphFieldSignal,
  SemanticModelGraphNodeSignal,
  SemanticModelGraphSignal,
} from '../../generated/signals'

type ModelNodeData = SemanticModelGraphNodeSignal & {
  selected: boolean
  dimmed: boolean
  base: boolean
  onSelect: (id: string) => void
}

type ModelEdgeData = SemanticModelGraphEdgeSignal & {
  selected: boolean
  sourceMarker: string
  targetMarker: string
}

type NodePosition = { x: number; y: number }

const NODE_WIDTH = 280
const HEADER_HEIGHT = 40
const FIELD_HEIGHT = 28
const NODE_GAP_X = 380
const NODE_GAP_Y = 92
const NODE_OFFSET_X = 72
const NODE_OFFSET_Y = 42

class SemanticModelGraphElement extends LitElement {
  @property({ type: Object }) graph: SemanticModelGraphSignal | null = null
  @property({ attribute: 'storagekey' }) storageKey = ''
  private root?: Root
  private mount?: HTMLDivElement
  private manualPositions = new Map<string, NodePosition>()
  private lastLayoutKey = ''

  createRenderRoot(): HTMLElement {
    return this
  }

  firstUpdated(): void {
    this.mount = this.renderRoot.querySelector('.semantic-model-graph-root') as HTMLDivElement | null ?? undefined
    if (this.mount) {
      this.root = createRoot(this.mount)
      this.renderFlow()
    }
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('graph') || changed.has('storageKey')) this.renderFlow()
  }

  disconnectedCallback(): void {
    this.root?.unmount()
    super.disconnectedCallback()
  }

  render() {
    return html`
      <style>${semanticModelGraphStyles}</style>
      <div class="semantic-model-graph-root"></div>
    `
  }

  private renderFlow(): void {
    if (!this.root) return
    const graph = this.resolvedGraph
    const layoutKey = graphLayoutKey(graph, this.storageKey)
    if (layoutKey !== this.lastLayoutKey) {
      this.manualPositions = loadLayout(layoutKey)
      this.lastLayoutKey = layoutKey
    }
    this.root.render(
      React.createElement(SemanticModelGraphFlow, {
        graph,
        layoutKey,
        manualPositions: this.manualPositions,
        onLayoutChange: (positions) => {
          this.manualPositions = positions
          saveLayout(layoutKey, positions)
        },
        onLayoutReset: () => {
          this.manualPositions = new Map()
          clearLayout(layoutKey)
        },
      }),
    )
  }

  private get resolvedGraph(): SemanticModelGraphSignal {
    return {
      baseTable: this.graph?.baseTable ?? '',
      nodes: this.graph?.nodes ?? [],
      edges: this.graph?.edges ?? [],
    }
  }
}

function SemanticModelGraphFlow({
  graph,
  layoutKey,
  manualPositions,
  onLayoutChange,
  onLayoutReset,
}: {
  graph: SemanticModelGraphSignal
  layoutKey: string
  manualPositions: Map<string, NodePosition>
  onLayoutChange: (positions: Map<string, NodePosition>) => void
  onLayoutReset: () => void
}) {
  const [selectedID, setSelectedID] = React.useState<string | undefined>(() => selectedNodeID(graph.nodes))
  const [nodes, setNodes, onNodesChange] = useNodesState<ModelNodeData>([])

  const selectedEdges = React.useMemo(() => relatedEdgeIDs(graph.edges, selectedID), [graph.edges, selectedID])

  React.useEffect(() => {
    setSelectedID((current) => selectedNodeID(graph.nodes, current))
  }, [graph.nodes, layoutKey])

  React.useEffect(() => {
    setNodes(graph.nodes.map((node) => toFlowNode(node, graph, selectedID, selectedEdges, manualPositions, setSelectedID)))
  }, [graph, layoutKey, manualPositions, setNodes])

  React.useEffect(() => {
    setNodes((currentNodes) => currentNodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        selected: node.id === selectedID,
        dimmed: nodeDimmed(node.id, graph.edges, selectedID, selectedEdges),
        base: node.id === graph.baseTable,
      },
    })))
  }, [graph.edges, selectedID, selectedEdges, setNodes])

  const edges = React.useMemo(() => graph.edges.map((edge) => toFlowEdge(edge, selectedEdges)), [graph.edges, selectedEdges])

  const saveDraggedLayout = React.useCallback((_event: unknown, node: Node<ModelNodeData>) => {
    const next = new Map(nodes.map((current) => [current.id, current.position] as [string, NodePosition]))
    next.set(node.id, node.position)
    onLayoutChange(next)
  }, [nodes, onLayoutChange])

  const resetLayout = React.useCallback(() => {
    onLayoutReset()
    setNodes(graph.nodes.map((node) => toFlowNode(node, graph, selectedID, selectedEdges, new Map(), setSelectedID)))
  }, [graph, onLayoutReset, selectedEdges, selectedID, setNodes])

  return React.createElement(
    'div',
    { className: 'semantic-model-graph-layout' },
    React.createElement(ReactFlow, {
      key: layoutKey,
      nodes,
      edges,
      onNodesChange,
      onNodeDragStop: saveDraggedLayout,
      nodeTypes: { modelTable: ModelTableNode },
      edgeTypes: { relationship: RelationshipEdge },
      fitView: true,
      fitViewOptions: { padding: 0.12 },
      minZoom: 0.2,
      maxZoom: 1.35,
      nodesDraggable: true,
      nodesConnectable: false,
      elementsSelectable: true,
      panOnDrag: true,
      zoomOnScroll: false,
      preventScrolling: false,
      children: [
        React.createElement(Background, { key: 'background', gap: 20, size: 1 }),
        React.createElement(Controls, { key: 'controls', showInteractive: false }),
        React.createElement(Panel, { key: 'layout-panel', position: 'top-right' },
          React.createElement(
            'button',
            {
              className: 'semantic-model-reset-button',
              type: 'button',
              title: 'Reset layout',
              'aria-label': 'Reset layout',
              onClick: resetLayout,
            },
            iconElement(RotateCcw, 'semantic-model-reset-icon'),
          ),
        ),
      ],
    }),
  )
}

function selectedNodeID(nodes: SemanticModelGraphNodeSignal[], current?: string): string | undefined {
  if (current && nodes.some((node) => node.id === current)) return current
  return nodes[0]?.id
}

function relatedEdgeIDs(edges: SemanticModelGraphEdgeSignal[], selected?: string): Set<string> {
  if (!selected) return new Set()
  return new Set(edges.filter((edge) => edge.source === selected || edge.target === selected).map((edge) => edge.id))
}

function toFlowNode(
  node: SemanticModelGraphNodeSignal,
  graph: SemanticModelGraphSignal,
  selectedID: string | undefined,
  selectedEdges: Set<string>,
  manualPositions: Map<string, NodePosition>,
  onSelect: (id: string) => void,
): Node<ModelNodeData> {
  const position = manualPositions.get(node.id) ?? modelNodePosition(node, graph)
  return {
    id: node.id,
    type: 'modelTable',
    position,
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: {
      ...node,
      selected: node.id === selectedID,
      dimmed: nodeDimmed(node.id, graph.edges, selectedID, selectedEdges),
      base: node.id === graph.baseTable,
      onSelect,
    },
  }
}

function nodeDimmed(id: string, edges: SemanticModelGraphEdgeSignal[], selectedID: string | undefined, selectedEdges: Set<string>): boolean {
  return Boolean(selectedID && id !== selectedID && !edges.some((edge) => selectedEdges.has(edge.id) && (edge.source === id || edge.target === id)))
}

function modelNodePosition(node: SemanticModelGraphNodeSignal, graph: SemanticModelGraphSignal): { x: number; y: number } {
  const ranks = modelNodeRanks(graph)
  const rank = ranks.get(node.id) ?? 0
  const rankNodes = graph.nodes
    .filter((candidate) => (ranks.get(candidate.id) ?? 0) === rank)
    .sort((left, right) => left.id.localeCompare(right.id))
  const row = rankNodes.findIndex((candidate) => candidate.id === node.id)
  return {
    x: NODE_OFFSET_X + (rank + minRankOffset(ranks)) * NODE_GAP_X,
    y: NODE_OFFSET_Y + Math.max(0, row) * (NODE_GAP_Y + nodeHeight(node)),
  }
}

function modelNodeRanks(graph: SemanticModelGraphSignal): Map<string, number> {
  const ranks = new Map<string, number>()
  const base = graph.baseTable && graph.nodes.some((node) => node.id === graph.baseTable) ? graph.baseTable : graph.nodes[0]?.id
  if (!base) return ranks
  ranks.set(base, 0)
  const queue = [base]
  while (queue.length) {
    const current = queue.shift() ?? ''
    const currentRank = ranks.get(current) ?? 0
    for (const edge of graph.edges) {
      const next = edge.source === current ? edge.target : edge.target === current ? edge.source : ''
      if (!next || ranks.has(next)) continue
      const direction = edge.source === current ? 1 : -1
      ranks.set(next, currentRank + direction)
      queue.push(next)
    }
  }
  for (const node of graph.nodes) {
    if (!ranks.has(node.id)) ranks.set(node.id, 0)
  }
  return ranks
}

function minRankOffset(ranks: Map<string, number>): number {
  const values = Array.from(ranks.values())
  if (!values.length) return 0
  const min = Math.min(...values)
  return min < 0 ? Math.abs(min) : 0
}

function nodeHeight(node: SemanticModelGraphNodeSignal): number {
  return HEADER_HEIGHT + Math.max(1, node.fields.length) * FIELD_HEIGHT + 12
}

function toFlowEdge(edge: SemanticModelGraphEdgeSignal, selectedEdges: Set<string>): Edge {
  const selected = selectedEdges.size === 0 || selectedEdges.has(edge.id)
  const [sourceMarker, targetMarker] = relationshipEndpointMarkers(edge.cardinality)
  return {
    id: edge.id,
    type: 'relationship',
    source: edge.source,
    target: edge.target,
    sourceHandle: `${edge.sourceField}:source`,
    targetHandle: `${edge.targetField}:target`,
    interactionWidth: 18,
    data: { ...edge, selected, sourceMarker, targetMarker },
    style: {
      stroke: edge.active ? 'var(--ld-fg-muted)' : 'color-mix(in srgb, var(--ld-fg-muted), transparent 42%)',
      strokeWidth: selected ? 2.2 : 1.4,
      strokeDasharray: edge.active ? undefined : '5 6',
      opacity: selected ? 0.92 : 0.18,
    },
  }
}

function RelationshipEdge(props: EdgeProps<Edge<ModelEdgeData>>) {
  const [path, labelX, labelY] = getBezierPath(props)
  const data = props.data
  const style = props.style ?? {}
  return React.createElement(React.Fragment, null,
    React.createElement('path', {
      id: props.id,
      className: 'react-flow__edge-path semantic-model-relationship-path',
      d: path,
      style,
    }),
    React.createElement(EdgeLabelRenderer, null,
      React.createElement('div', {
        className: `semantic-model-edge-label ${data?.selected ? 'selected' : ''}`,
        style: {
          transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
        },
      }, data?.label ?? ''),
      React.createElement('div', {
        className: 'semantic-model-edge-endpoint source',
        style: {
          transform: `translate(-50%, -50%) translate(${props.sourceX}px,${props.sourceY}px)`,
        },
      }, data?.sourceMarker ?? ''),
      React.createElement('div', {
        className: 'semantic-model-edge-endpoint target',
        style: {
          transform: `translate(-50%, -50%) translate(${props.targetX}px,${props.targetY}px)`,
        },
      }, data?.targetMarker ?? ''),
    ),
  )
}

function relationshipEndpointMarkers(cardinality: string): [string, string] {
  switch (cardinality) {
    case 'many_to_one':
      return ['*', '1']
    case 'one_to_one':
      return ['1', '1']
    case 'one_to_many':
      return ['1', '*']
    case 'many_to_many':
      return ['*', '*']
    default:
      return ['', '']
  }
}

function graphLayoutKey(graph: SemanticModelGraphSignal, storageKey: string): string {
  const nodePart = graph.nodes.map((node) => `${node.id}:${node.fields.map((field) => field.name).join(',')}`).join('|')
  const edgePart = graph.edges.map((edge) => `${edge.id}:${edge.source}.${edge.sourceField}->${edge.target}.${edge.targetField}:${edge.cardinality}`).join('|')
  return `libredash:semantic-model-graph:v1:${storageKey || graph.baseTable || 'model'}:${nodePart}:${edgePart}`
}

function loadLayout(key: string): Map<string, NodePosition> {
  try {
    const raw = globalThis.localStorage?.getItem(key)
    if (!raw) return new Map()
    const parsed = JSON.parse(raw) as Record<string, NodePosition>
    return new Map(Object.entries(parsed).filter((entry): entry is [string, NodePosition] => (
      Number.isFinite(entry[1]?.x) && Number.isFinite(entry[1]?.y)
    )))
  } catch {
    return new Map()
  }
}

function saveLayout(key: string, positions: Map<string, NodePosition>): void {
  try {
    globalThis.localStorage?.setItem(key, JSON.stringify(Object.fromEntries(positions)))
  } catch {
    // Layout persistence is progressive enhancement only.
  }
}

function clearLayout(key: string): void {
  try {
    globalThis.localStorage?.removeItem(key)
  } catch {
    // Layout persistence is progressive enhancement only.
  }
}

function ModelTableNode({ data }: { data: ModelNodeData }) {
  const className = [
    'semantic-model-node',
    data.selected ? 'semantic-model-node-selected' : '',
    data.dimmed ? 'semantic-model-node-dimmed' : '',
  ].filter(Boolean).join(' ')
  const select = () => data.onSelect(data.id)
  return React.createElement(
    'div',
    {
      className,
      role: 'button',
      tabIndex: 0,
      'aria-label': `Model table ${data.title}`,
      'aria-pressed': data.selected ? 'true' : 'false',
      onClick: select,
      onKeyDown: (event: React.KeyboardEvent) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        select()
      },
    },
    React.createElement('div', { className: 'semantic-model-node-header' },
      React.createElement('div', { className: 'semantic-model-node-title', title: data.base ? `${data.title}, base table` : data.title },
        iconElement(Table2, 'semantic-model-table-icon'),
        React.createElement('span', { className: data.base ? 'semantic-model-node-title-base' : '' }, data.title),
        data.base ? React.createElement('span', { className: 'semantic-model-node-base-text' }, '\u00b7 base table') : null,
      ),
    ),
    React.createElement('div', { className: 'semantic-model-node-fields' },
      data.fields.map((field, index) => React.createElement(ModelFieldRow, { key: field.name, field, index })),
    ),
  )
}

function ModelFieldRow({ field, index }: { field: SemanticModelGraphFieldSignal; index: number }) {
  const top = HEADER_HEIGHT + index * FIELD_HEIGHT + FIELD_HEIGHT / 2
  const className = [
    'semantic-model-field',
    field.join ? 'semantic-model-field-join' : '',
    field.primaryKey ? 'semantic-model-field-primary' : '',
  ].filter(Boolean).join(' ')
  return React.createElement(
    'div',
    { className },
    field.join ? React.createElement(Handle, { id: `${field.name}:target`, type: 'target', position: Position.Left, style: { top } }) : null,
    React.createElement('span', { className: 'semantic-model-field-type-icon', title: field.type ? `Column type ${field.type}` : 'Column type unknown' }, iconElement(fieldTypeIcon(field.type), 'semantic-model-type-icon')),
    React.createElement('span', { className: 'semantic-model-field-name', title: field.primaryKey ? `${field.name} (primary key)` : field.name }, field.name),
    field.primaryKey ? React.createElement('span', { className: 'semantic-model-field-key', title: 'Primary key' }, 'PK') : null,
    field.join ? React.createElement(Handle, { id: `${field.name}:source`, type: 'source', position: Position.Right, style: { top } }) : null,
  )
}

function fieldTypeIcon(type = ''): IconNode {
  const normalized = type.toLowerCase()
  if (normalized.includes('time')) return Clock3
  if (normalized.includes('date')) return CalendarDays
  if (normalized.includes('bool')) return ToggleLeft
  if (normalized.includes('int') || normalized.includes('decimal') || normalized.includes('double') || normalized.includes('float') || normalized.includes('number') || normalized.includes('numeric')) return Hash
  if (normalized.includes('blob') || normalized.includes('binary') || normalized.includes('byte')) return Binary
  if (normalized.includes('json') || normalized.includes('struct') || normalized.includes('map') || normalized.includes('list') || normalized.includes('array')) return Braces
  if (normalized.includes('char') || normalized.includes('text') || normalized.includes('string') || normalized.includes('uuid')) return Type
  return CircleHelp
}

function iconElement(icon: IconNode, className: string) {
  return React.createElement(
    'svg',
    {
      className,
      xmlns: 'http://www.w3.org/2000/svg',
      width: 14,
      height: 14,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      'aria-hidden': 'true',
    },
    icon.map(([tag, attrs], index) => React.createElement(tag, { ...attrs, key: index })),
  )
}

const semanticModelGraphStyles = `
  ld-semantic-model-graph,
  ld-semantic-model-graph .semantic-model-graph-root,
  ld-semantic-model-graph .semantic-model-graph-layout {
    display: block;
    height: 100%;
    min-width: 0;
    min-height: 0;
  }

  ld-semantic-model-graph .semantic-model-graph-layout {
    background:
      linear-gradient(var(--ld-bg-page, var(--ld-bg-app)), var(--ld-bg-page, var(--ld-bg-app))),
      radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--ld-fg-muted), transparent 88%) 1px, transparent 0);
    background-size: auto, 20px 20px;
  }

  ld-semantic-model-graph .react-flow {
    position: relative;
    overflow: hidden;
    width: 100%;
    height: 100%;
    color: var(--ld-fg-default);
    background-color: transparent;
  }

  ld-semantic-model-graph .react-flow__container,
  ld-semantic-model-graph .react-flow__renderer,
  ld-semantic-model-graph .react-flow__viewport,
  ld-semantic-model-graph .react-flow__pane,
  ld-semantic-model-graph .react-flow__nodes,
  ld-semantic-model-graph .react-flow .react-flow__edges,
  ld-semantic-model-graph .react-flow .react-flow__edges svg {
    position: absolute;
  }

  ld-semantic-model-graph .react-flow__container,
  ld-semantic-model-graph .react-flow__viewport {
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    transform-origin: 0 0;
  }

  ld-semantic-model-graph .react-flow__node {
    position: absolute;
    box-sizing: border-box;
    pointer-events: all;
    transform-origin: 0 0;
    user-select: none;
  }

  ld-semantic-model-graph .react-flow .react-flow__edges svg {
    overflow: visible;
    pointer-events: none;
  }

  ld-semantic-model-graph .react-flow__edge-path,
  ld-semantic-model-graph .react-flow__connection-path {
    fill: none;
  }

  ld-semantic-model-graph .semantic-model-relationship-path {
    pointer-events: visibleStroke;
  }

  ld-semantic-model-graph .react-flow__edgelabel-renderer {
    position: absolute;
    width: 100%;
    height: 100%;
    pointer-events: none;
    user-select: none;
  }

  ld-semantic-model-graph .react-flow__background {
    pointer-events: none;
    z-index: -1;
  }

  ld-semantic-model-graph .react-flow__handle {
    position: absolute;
    width: 8px;
    height: 8px;
    min-width: 8px;
    min-height: 8px;
    border: 1px solid var(--ld-bg-panel);
    border-radius: 50%;
    background: var(--ld-fg-muted);
    pointer-events: none;
  }

  ld-semantic-model-graph .react-flow__handle-left {
    left: 0;
    transform: translate(-50%, -50%);
  }

  ld-semantic-model-graph .react-flow__handle-right {
    right: 0;
    transform: translate(50%, -50%);
  }

  ld-semantic-model-graph .react-flow__panel {
    position: absolute;
    z-index: 5;
    margin: var(--base-size-16);
  }

  ld-semantic-model-graph .react-flow__panel.left {
    left: 0;
  }

  ld-semantic-model-graph .react-flow__panel.bottom {
    bottom: 0;
  }

  ld-semantic-model-graph .react-flow__controls {
    display: flex;
    flex-direction: column;
    border: var(--ld-border-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small, none);
  }

  ld-semantic-model-graph .react-flow__controls-button {
    display: flex;
    width: 26px;
    height: 26px;
    align-items: center;
    justify-content: center;
    border: 0;
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
    color: var(--ld-fg-default);
    padding: 4px;
    cursor: pointer;
  }

  ld-semantic-model-graph .react-flow__controls-button svg {
    width: 100%;
    max-width: 12px;
    max-height: 12px;
    fill: currentColor;
  }

  ld-semantic-model-graph .semantic-model-reset-button {
    display: inline-flex;
    width: 28px;
    height: 28px;
    align-items: center;
    justify-content: center;
    border: var(--ld-border-default);
    border-radius: var(--ld-radius-tight);
    background: var(--ld-bg-panel);
    color: var(--ld-fg-default);
    padding: 0;
    font: inherit;
    cursor: pointer;
  }

  ld-semantic-model-graph .semantic-model-reset-button:hover,
  ld-semantic-model-graph .semantic-model-reset-button:focus-visible {
    background: var(--ld-bg-control-hover, var(--ld-bg-panel-muted));
    outline: 0;
  }

  ld-semantic-model-graph .semantic-model-reset-icon {
    display: block;
    width: 14px;
    height: 14px;
  }

  ld-semantic-model-graph .react-flow__attribution {
    display: none;
  }

  ld-semantic-model-graph .semantic-model-edge-label,
  ld-semantic-model-graph .semantic-model-edge-endpoint {
    position: absolute;
    display: inline-grid;
    place-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-full);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small, none);
    color: var(--ld-fg-default);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    line-height: 1;
    pointer-events: none;
  }

  ld-semantic-model-graph .semantic-model-edge-label {
    min-width: 30px;
    min-height: 20px;
    padding: 0 var(--base-size-6);
  }

  ld-semantic-model-graph .semantic-model-edge-label.selected {
    border-color: var(--ld-fg-muted);
    color: var(--ld-fg-default);
  }

  ld-semantic-model-graph .semantic-model-edge-endpoint {
    width: 20px;
    height: 20px;
    border-color: color-mix(in srgb, var(--ld-fg-muted), transparent 35%);
    color: var(--ld-fg-default);
  }

  ld-semantic-model-graph .semantic-model-edge-endpoint.source {
    margin-left: -16px;
  }

  ld-semantic-model-graph .semantic-model-edge-endpoint.target {
    margin-left: 16px;
  }

  ld-semantic-model-graph .semantic-model-node {
    width: ${NODE_WIDTH}px;
    overflow: hidden;
    border: var(--borderWidth-default) solid var(--ld-line-muted);
    border-radius: var(--borderRadius-default);
    background: var(--ld-bg-panel);
    box-shadow: var(--shadow-resting-small, none);
    color: var(--ld-fg-default);
    cursor: pointer;
  }

  ld-semantic-model-graph .semantic-model-node-selected {
    border-color: var(--ld-fg-muted);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--ld-fg-muted), transparent 35%), var(--shadow-resting-small, none);
  }

  ld-semantic-model-graph .semantic-model-node-dimmed {
    opacity: 0.42;
  }

  ld-semantic-model-graph .semantic-model-node:focus-visible {
    outline: 2px solid var(--ld-fg-muted);
    outline-offset: 2px;
  }

  ld-semantic-model-graph .semantic-model-node-header {
    display: flex;
    min-height: ${HEADER_HEIGHT}px;
    min-width: 0;
    gap: var(--base-size-8);
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
    padding: 0 var(--base-size-12);
    align-items: center;
    justify-content: space-between;
  }

  ld-semantic-model-graph .semantic-model-node-title {
    display: inline-flex;
    min-width: 0;
    align-items: center;
    gap: var(--base-size-6);
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-strong);
    line-height: var(--ld-line-height-tight);
  }

  ld-semantic-model-graph .semantic-model-node-title span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  ld-semantic-model-graph .semantic-model-node-title-base {
    font-style: italic;
  }

  ld-semantic-model-graph .semantic-model-node-base-text {
    flex: 0 0 auto;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-normal);
  }

  ld-semantic-model-graph .semantic-model-table-icon {
    flex: 0 0 auto;
    color: var(--ld-fg-muted);
  }

  ld-semantic-model-graph .semantic-model-node-fields {
    display: grid;
  }

  ld-semantic-model-graph .semantic-model-field {
    display: grid;
    min-height: ${FIELD_HEIGHT}px;
    grid-template-columns: 18px minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--base-size-6);
    border-bottom: var(--ld-border-muted);
    box-shadow: inset 0 0 0 0 transparent;
    padding: 0 var(--ld-space-control);
  }

  ld-semantic-model-graph .semantic-model-field:last-child {
    border-bottom: 0;
  }

  ld-semantic-model-graph .semantic-model-field-join {
    background: color-mix(in srgb, var(--ld-fg-muted), transparent 90%);
    box-shadow: inset 2px 0 0 color-mix(in srgb, var(--ld-fg-muted), transparent 42%);
  }

  ld-semantic-model-graph .semantic-model-field-name {
    overflow: hidden;
    color: var(--ld-fg-default);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--ld-font-family-mono, ui-monospace, SFMono-Regular, Consolas, monospace);
    font-size: var(--ld-font-size-caption);
  }

  ld-semantic-model-graph .semantic-model-field-primary .semantic-model-field-name {
    font-weight: var(--ld-font-weight-strong);
  }

  ld-semantic-model-graph .semantic-model-field-type-icon {
    display: inline-grid;
    width: 18px;
    height: 18px;
    place-items: center;
    color: var(--ld-fg-muted);
  }

  ld-semantic-model-graph .semantic-model-type-icon {
    display: block;
    width: 14px;
    height: 14px;
  }

  ld-semantic-model-graph .semantic-model-field-key {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    line-height: 1;
  }
`

if (!customElements.get('ld-semantic-model-graph')) customElements.define('ld-semantic-model-graph', SemanticModelGraphElement)
