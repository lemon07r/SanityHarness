import { describe, it } from "node:test";
import assert from "node:assert";
import { InputCell, ComputeCell } from "./react.ts";

describe("React (hidden)", () => {
  it("propagates through a deep dependency chain", () => {
    const input = new InputCell(1);
    const a = new ComputeCell([input], (x: number) => x + 1);
    const b = new ComputeCell([a] as unknown as InputCell<unknown>[], (x: number) => x * 2);
    const c = new ComputeCell([b] as unknown as InputCell<unknown>[], (x: number) => x - 3);

    assert.strictEqual(c.value, ((1 + 1) * 2) - 3);
    input.setValue(5);
    assert.strictEqual(c.value, ((5 + 1) * 2) - 3);
  });

  it("fan-out/fan-in callbacks fire once per propagation", () => {
    const input = new InputCell(2);
    const left = new ComputeCell([input], (x: number) => x + 1);
    const right = new ComputeCell([input], (x: number) => x * 3);
    const bottom = new ComputeCell(
      [left, right] as unknown as InputCell<unknown>[],
      (l: number, r: number) => l + r
    );

    const values: number[] = [];
    bottom.addCallback((v) => values.push(v));
    input.setValue(4);

    assert.strictEqual(bottom.value, (4 + 1) + (4 * 3));
    assert.deepStrictEqual(values, [bottom.value]);
  });

  it("does not fire callback when final value is unchanged", () => {
    const input = new InputCell(1);
    const parity = new ComputeCell([input], (x: number) => x % 2);
    const out = new ComputeCell(
      [parity] as unknown as InputCell<unknown>[],
      (x: number) => x + 10
    );

    const values: number[] = [];
    out.addCallback((v) => values.push(v));

    input.setValue(3); // parity unchanged
    assert.deepStrictEqual(values, []);
  });
});
