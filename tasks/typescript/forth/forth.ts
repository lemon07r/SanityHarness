export class Forth {
  private stack: number[] = [];

  evaluate(input: string): void {
    throw new Error("Please implement the evaluate method");
  }

  get stackValue(): number[] {
    return [...this.stack];
  }
}

export class ValueError extends Error {
  constructor(message?: string) {
    super(message ?? "Invalid operation");
    this.name = "ValueError";
  }
}
