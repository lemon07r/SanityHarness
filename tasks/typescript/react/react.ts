type Callback<T> = (value: T) => void;

// Propagation context for batch updates
const propagationContext: {
  isPropagating: boolean;
  firedCallbacks: Set<string>;
  callbacksToFire: Array<{ cell: ComputeCell<unknown>; callback: Callback<unknown> }>;
} = {
  isPropagating: false,
  firedCallbacks: new Set(),
  callbacksToFire: [],
};

export class InputCell<T> {
  value: T;
  private dependents: Set<ComputeCell<unknown>> = new Set();

  constructor(value: T) {
    this.value = value;
  }

  setValue(value: T): void {
    if (this.value !== value) {
      this.value = value;
      this.notifyDependents();
    }
  }

  addDependent(cell: ComputeCell<unknown>): void {
    this.dependents.add(cell);
  }

  removeDependent(cell: ComputeCell<unknown>): void {
    this.dependents.delete(cell);
  }

  private notifyDependents(): void {
    if (propagationContext.isPropagating) {
      return;
    }

    propagationContext.isPropagating = true;
    propagationContext.firedCallbacks.clear();
    propagationContext.callbacksToFire = [];

    // Collect all compute cells that need updating
    const cellsToUpdate: ComputeCell<unknown>[] = [];
    const visited = new Set<ComputeCell<unknown>>();

    for (const cell of this.dependents) {
      collectDependents(cell, cellsToUpdate, visited);
    }

    // Sort by dependency depth (cells with fewer dependencies first)
    cellsToUpdate.sort((a, b) => a.dependencyDepth - b.dependencyDepth);

    // Update all cell values
    for (const cell of cellsToUpdate) {
      cell.recalculate();
    }

    // Fire all callbacks
    for (const { cell, callback } of propagationContext.callbacksToFire) {
      callback(cell.value as never);
    }

    propagationContext.isPropagating = false;
    propagationContext.firedCallbacks.clear();
    propagationContext.callbacksToFire = [];
  }
}

function collectDependents(
  cell: ComputeCell<unknown>,
  result: ComputeCell<unknown>[],
  visited: Set<ComputeCell<unknown>>
): void {
  if (!visited.has(cell)) {
    visited.add(cell);
    result.push(cell);
    for (const dependent of cell.dependents) {
      collectDependents(dependent, result, visited);
    }
  }
}

export class ComputeCell<T> {
  private callbacks: Map<Callback<T>, string> = new Map();
  private callbackIdCounter = 0;
  private dependents: Set<ComputeCell<unknown>> = new Set();
  value: T;
  readonly dependencyDepth: number;

  constructor(
    inputCells: InputCell<unknown>[],
    computeFn: (...values: unknown[]) => T
  ) {
    this.value = computeFn(
      ...inputCells.map((cell) => (cell as InputCell<unknown>).value)
    );

    // Calculate dependency depth for topological sorting
    let maxDepth = 0;
    for (const cell of inputCells) {
      if (cell instanceof ComputeCell) {
        maxDepth = Math.max(maxDepth, cell.dependencyDepth + 1);
      }
    }
    this.dependencyDepth = maxDepth;

    // Register as dependent of all input cells
    for (const cell of inputCells) {
      cell.addDependent(this);
    }

    // Store the compute function and inputs for recalculation
    (this as unknown as { computeFn: (...values: unknown[]) => T }).computeFn = computeFn;
    (this as unknown as { inputs: InputCell<unknown>[] }).inputs = inputCells;
  }

  get value(): T {
    return this.value;
  }

  addCallback(callback: Callback<T>): void {
    const id = `cb-${++this.callbackIdCounter}`;
    this.callbacks.set(callback, id);

    if (propagationContext.isPropagating) {
      // If we're in the middle of propagation, fire immediately
      // since we know the value has settled
      propagationContext.callbacksToFire.push({ cell: this as unknown as ComputeCell<unknown>, callback: callback as Callback<unknown> });
    }
  }

  addDependent(cell: ComputeCell<unknown>): void {
    this.dependents.add(cell);
  }

  removeDependent(cell: ComputeCell<unknown>): void {
    this.dependents.delete(cell);
  }

  removeCallback(callback: Callback<T>): void {
    this.callbacks.delete(callback);
  }

  recalculate(): void {
    const inputs = (this as unknown as { inputs: InputCell<unknown>[] }).inputs;
    const computeFn = (this as unknown as { computeFn: (...values: unknown[]) => T }).computeFn;
    const newValue = computeFn(...inputs.map((cell) => cell.value));
    const oldValue = this.value;
    this.value = newValue;

    // Check if value changed and schedule callbacks
    if (!Object.is(newValue, oldValue)) {
      for (const [callback, id] of this.callbacks) {
        if (!propagationContext.firedCallbacks.has(id)) {
          propagationContext.firedCallbacks.add(id);
          propagationContext.callbacksToFire.push({ cell: this as unknown as ComputeCell<unknown>, callback: callback as Callback<unknown> });
        }
      }
    }
  }
}
