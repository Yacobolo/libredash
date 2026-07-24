import type {
  DashboardAppliedFilterState,
  DashboardFilterCommand,
  DashboardFilterExpression,
  DashboardFilterState,
} from '../../../generated/signals'

type CommandSink = (command: DashboardFilterCommand) => void
type MutationIDFactory = () => string

type WithoutBaseRevision<T> = T extends { baseRevision: number } ? Omit<T, 'baseRevision'> : never
type PendingCommand = WithoutBaseRevision<DashboardFilterCommand>
type MutateCommand = Extract<DashboardFilterCommand | PendingCommand, { kind: 'mutate' }>

const emptyState: DashboardFilterState = {
  revision: 0,
  appliedControls: {},
  draftControls: {},
  dirtyBindings: [],
  defaultsRevision: '',
}

export class DashboardFilterController {
  private canonical: DashboardFilterState = cloneState(emptyState)
  private optimistic: DashboardFilterState = cloneState(emptyState)
  private queue: PendingCommand[] = []
  private inFlight: DashboardFilterCommand | null = null
  private mode: 'immediate' | 'deferred' = 'immediate'
  private defaults: Record<string, DashboardFilterExpression> = {}

  constructor(
    private readonly send: CommandSink,
    private readonly mutationID: MutationIDFactory = () => crypto.randomUUID(),
  ) {}

  setApplicationMode(mode: 'immediate' | 'deferred') {
    this.mode = mode
  }

  setDefaults(defaults: Record<string, DashboardFilterExpression>) {
    this.defaults = Object.fromEntries(
      Object.entries(defaults).map(([key, expression]) => [key, cloneExpression(expression)]),
    )
  }

  reconcile(state: DashboardFilterState) {
    this.canonical = cloneState(state)
    this.optimistic = cloneState(state)
    this.inFlight = null
    this.projectQueued()
    this.flush()
  }

  reject(clientMutationID: string, state: DashboardFilterState): boolean {
    if (!this.inFlight || this.inFlight.clientMutationID !== clientMutationID) return false
    this.reconcile(state)
    return true
  }

  get projected(): DashboardFilterState {
    return cloneState(this.optimistic)
  }

  get pending(): boolean {
    return this.inFlight !== null || this.queue.length > 0
  }

  expression(bindingKey: string): DashboardFilterExpression {
    return cloneExpression(
      this.optimistic.draftControls[bindingKey]
      ?? this.optimistic.appliedControls[bindingKey]?.expression
      ?? { kind: 'unfiltered' },
    )
  }

  mutate(bindingKey: string, expression: DashboardFilterExpression) {
    this.enqueue({
      kind: 'mutate',
      clientMutationID: this.mutationID(),
      bindingKey,
      operation: 'set',
      expression: cloneExpression(expression),
    })
  }

  clear(bindingKey: string) {
    this.enqueue({
      kind: 'mutate',
      clientMutationID: this.mutationID(),
      bindingKey,
      operation: 'clear',
    })
  }

  resetBinding(bindingKey: string) {
    this.enqueue({
      kind: 'mutate',
      clientMutationID: this.mutationID(),
      bindingKey,
      operation: 'reset_binding',
    })
  }

  apply() {
    this.enqueue({ kind: 'apply', clientMutationID: this.mutationID() })
  }

  cancel() {
    this.enqueue({ kind: 'cancel', clientMutationID: this.mutationID() })
  }

  reset(scope: 'page' | 'dashboard', bindingKeys: string[]) {
    this.enqueue({
      kind: 'reset',
      clientMutationID: this.mutationID(),
      resetScope: scope,
      bindingKeys: [...bindingKeys],
    })
  }

  private enqueue(command: PendingCommand) {
    this.queue.push(command)
    this.projectCommand(command)
    this.flush()
  }

  private flush() {
    if (this.inFlight || this.queue.length === 0) return
    const pending = this.queue.shift()
    if (!pending) return
    const command = { ...pending, baseRevision: this.canonical.revision } as DashboardFilterCommand
    this.inFlight = command
    this.send(command)
  }

  private projectQueued() {
    if (this.inFlight) this.projectCommand(this.inFlight)
    for (const command of this.queue) this.projectCommand(command)
  }

  private projectCommand(command: PendingCommand | DashboardFilterCommand) {
    if (command.kind === 'mutate' && command.bindingKey) {
      const expression = command.operation === 'reset_binding'
        ? cloneExpression(this.defaults[command.bindingKey] ?? { kind: 'unfiltered' })
        : optimisticExpression(command)
      if (this.mode === 'deferred') {
        this.optimistic.draftControls[command.bindingKey] = expression
        if (!this.optimistic.dirtyBindings.includes(command.bindingKey)) {
          this.optimistic.dirtyBindings = [...this.optimistic.dirtyBindings, command.bindingKey].sort()
        }
        return
      }
      const current = this.optimistic.appliedControls[command.bindingKey]
      this.optimistic.appliedControls[command.bindingKey] = optimisticApplied(current, expression)
      return
    }
    if (command.kind === 'reset') {
      for (const bindingKey of command.bindingKeys) {
        const expression = cloneExpression(this.defaults[bindingKey] ?? { kind: 'unfiltered' })
        if (this.mode === 'deferred') {
          this.optimistic.draftControls[bindingKey] = expression
          if (!this.optimistic.dirtyBindings.includes(bindingKey)) {
            this.optimistic.dirtyBindings = [...this.optimistic.dirtyBindings, bindingKey].sort()
          }
          continue
        }
        this.optimistic.appliedControls[bindingKey] = optimisticApplied(
          this.optimistic.appliedControls[bindingKey],
          expression,
        )
      }
      return
    }
    if (command.kind === 'cancel') {
      this.optimistic.draftControls = {}
      this.optimistic.dirtyBindings = []
      return
    }
    if (command.kind === 'apply') {
      for (const bindingKey of this.optimistic.dirtyBindings) {
        const expression = this.optimistic.draftControls[bindingKey]
        if (!expression) continue
        this.optimistic.appliedControls[bindingKey] = optimisticApplied(
          this.optimistic.appliedControls[bindingKey],
          expression,
        )
      }
      this.optimistic.draftControls = {}
      this.optimistic.dirtyBindings = []
    }
  }
}

function optimisticExpression(
  command: MutateCommand,
): DashboardFilterExpression {
  switch (command.operation) {
    case 'set':
      return cloneExpression(command.expression ?? { kind: 'unfiltered' })
    case 'clear':
      return { kind: 'unfiltered' }
    default:
      return { kind: 'unfiltered' }
  }
}

function optimisticApplied(
  current: DashboardAppliedFilterState | undefined,
  expression: DashboardFilterExpression,
): DashboardAppliedFilterState {
  return {
    expression: cloneExpression(expression),
    resolvedExpression: cloneExpression(expression),
    evaluatedAt: current?.evaluatedAt,
  }
}

function cloneState(state: DashboardFilterState): DashboardFilterState {
  return {
    revision: state.revision,
    defaultsRevision: state.defaultsRevision,
    dirtyBindings: [...(state.dirtyBindings ?? [])],
    appliedControls: Object.fromEntries(
      Object.entries(state.appliedControls ?? {}).map(([key, applied]) => [key, {
        expression: cloneExpression(applied.expression),
        resolvedExpression: cloneExpression(applied.resolvedExpression),
        ...(applied.evaluatedAt ? { evaluatedAt: applied.evaluatedAt } : {}),
      }]),
    ),
    draftControls: Object.fromEntries(
      Object.entries(state.draftControls ?? {}).map(([key, expression]) => [key, cloneExpression(expression)]),
    ),
  }
}

function cloneExpression(expression: DashboardFilterExpression): DashboardFilterExpression {
  return JSON.parse(JSON.stringify(expression)) as DashboardFilterExpression
}
