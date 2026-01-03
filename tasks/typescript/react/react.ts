type Callback<T> = (value: T) => void;

export class InputCell<T> {
  constructor(public value: T) {
    throw new Error("Please implement InputCell");
  }

  setValue(value: T): void {
    throw new Error("Please implement setValue");
  }
}

export class ComputeCell<T> {
  constructor(
    inputCells: InputCell<unknown>[],
    computeFn: (...values: unknown[]) => T
  ) {
    throw new Error("Please implement ComputeCell");
  }

  get value(): T {
    throw new Error("Please implement value getter");
  }

  addCallback(callback: Callback<T>): void {
    throw new Error("Please implement addCallback");
  }

  removeCallback(callback: Callback<T>): void {
    throw new Error("Please implement removeCallback");
  }
}
