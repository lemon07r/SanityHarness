import { describe, it } from "node:test";
import assert from "node:assert";
import { InputCell, ComputeCell } from "./react.ts";

describe("React", () => {
  describe("InputCell", () => {
    it("has a value", () => {
      const cell = new InputCell(10);
      assert.strictEqual(cell.value, 10);
    });

    it("can set value", () => {
      const cell = new InputCell(10);
      cell.setValue(20);
      assert.strictEqual(cell.value, 20);
    });
  });

  describe("ComputeCell", () => {
    it("computes initial value from input", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x + 1);
      assert.strictEqual(output.value, 2);
    });

    it("updates when input changes", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x + 1);
      input.setValue(3);
      assert.strictEqual(output.value, 4);
    });

    it("can depend on multiple inputs", () => {
      const a = new InputCell(1);
      const b = new InputCell(2);
      const sum = new ComputeCell([a, b], (x: number, y: number) => x + y);
      assert.strictEqual(sum.value, 3);
    });

    it("can chain compute cells", () => {
      const input = new InputCell(1);
      const times2 = new ComputeCell([input], (x: number) => x * 2);
      const times30 = new ComputeCell([input], (x: number) => x * 30);
      const sum = new ComputeCell(
        [times2, times30] as unknown as InputCell<unknown>[],
        (x: number, y: number) => x + y
      );
      assert.strictEqual(sum.value, 32);
      input.setValue(3);
      assert.strictEqual(sum.value, 96);
    });
  });

  describe("callbacks", () => {
    it("fires callback on change", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x + 1);
      const values: number[] = [];
      output.addCallback((v) => values.push(v));
      input.setValue(3);
      assert.deepStrictEqual(values, [4]);
    });

    it("does not fire callback when value unchanged", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => (x < 3 ? 1 : 2));
      const values: number[] = [];
      output.addCallback((v) => values.push(v));
      input.setValue(2);
      assert.deepStrictEqual(values, []);
    });

    it("can remove callback", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x + 1);
      const values: number[] = [];
      const callback = (v: number) => values.push(v);
      output.addCallback(callback);
      input.setValue(2);
      output.removeCallback(callback);
      input.setValue(3);
      assert.deepStrictEqual(values, [3]);
    });

    it("callbacks only fire once per propagation", () => {
      const input = new InputCell(1);
      const a = new ComputeCell([input], (x: number) => x + 1);
      const b = new ComputeCell([input], (x: number) => x - 1);
      const c = new ComputeCell(
        [a, b] as unknown as InputCell<unknown>[],
        (x: number, y: number) => x * y
      );
      const values: number[] = [];
      c.addCallback((v) => values.push(v));
      input.setValue(4);
      assert.deepStrictEqual(values, [15]);
    });
  });
});
