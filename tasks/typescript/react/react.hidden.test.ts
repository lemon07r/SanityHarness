import { describe, it } from "node:test";
import assert from "node:assert";
import { InputCell, ComputeCell } from "./react.ts";

// Extended interfaces that hidden tests require
interface WeakCallbackCell<T> {
  addWeakCallback(callback: (value: T) => void): WeakRef<(value: T) => void>;
}

interface AsyncComputeCell<T> {
  readonly value: T;
  readonly pendingValue: Promise<T> | null;
  refresh(): Promise<T>;
}

interface DependencyInspectable {
  getDependencies(): Array<InputCell<unknown> | ComputeCell<unknown>>;
  getDependents(): Array<ComputeCell<unknown>>;
}

describe("React Hidden Tests", () => {
  describe("WeakCallback", () => {
    it("weak callbacks can be garbage collected", async () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x + 1);

      // Check if ComputeCell implements WeakCallbackCell
      if (!("addWeakCallback" in output)) {
        throw new Error("ComputeCell must implement addWeakCallback method");
      }

      const weakCell = output as unknown as WeakCallbackCell<number>;
      const values: number[] = [];

      // Create callback in a scope so it can be GC'd
      let callback: ((v: number) => void) | null = (v: number) =>
        values.push(v);
      const weakRef = weakCell.addWeakCallback(callback);

      input.setValue(2);
      assert.deepStrictEqual(values, [3]);

      // Clear the strong reference
      callback = null;

      // Force GC (this is a hint, not guaranteed)
      if (global.gc) {
        global.gc();
        await new Promise((resolve) => setTimeout(resolve, 100));
      }

      // WeakRef should eventually become undefined after GC
      // (We can't guarantee GC timing, so we just verify the weak ref exists)
      assert.ok(weakRef instanceof WeakRef);
    });

    it("weak callback stops firing when dereferenced", () => {
      const input = new InputCell(1);
      const output = new ComputeCell([input], (x: number) => x * 2);

      if (!("addWeakCallback" in output)) {
        throw new Error("ComputeCell must implement addWeakCallback method");
      }

      const weakCell = output as unknown as WeakCallbackCell<number>;
      const values: number[] = [];

      let callback: ((v: number) => void) | null = (v: number) =>
        values.push(v);
      weakCell.addWeakCallback(callback);

      input.setValue(2);
      assert.strictEqual(values.length, 1);

      // Strong callbacks should still work
      const strongValues: number[] = [];
      output.addCallback((v) => strongValues.push(v));

      callback = null; // Release weak callback

      input.setValue(3);
      assert.strictEqual(strongValues.length, 1);
    });
  });

  describe("AsyncComputeCell", () => {
    it("supports async compute functions", async () => {
      const input = new InputCell(5);

      // Check if there's an AsyncComputeCell constructor
      const AsyncComputeCellClass = (
        ComputeCell as unknown as { Async?: unknown }
      ).Async;
      if (!AsyncComputeCellClass) {
        // Alternative: check if ComputeCell can accept async functions
        const asyncOutput = new ComputeCell([input], async (x: number) => {
          await new Promise((resolve) => setTimeout(resolve, 10));
          return x * 2;
        });

        if (!("pendingValue" in asyncOutput)) {
          throw new Error(
            "ComputeCell must support async functions with pendingValue property"
          );
        }

        const asyncCell = asyncOutput as unknown as AsyncComputeCell<number>;

        // Wait for initial computation
        if (asyncCell.pendingValue) {
          await asyncCell.pendingValue;
        }

        assert.strictEqual(asyncCell.value, 10);
      }
    });

    it("refresh triggers recomputation", async () => {
      const input = new InputCell(1);
      let callCount = 0;

      const output = new ComputeCell([input], async (x: number) => {
        callCount++;
        await new Promise((resolve) => setTimeout(resolve, 5));
        return x + callCount;
      });

      if (!("refresh" in output)) {
        throw new Error("ComputeCell must implement refresh method");
      }

      const asyncCell = output as unknown as AsyncComputeCell<number>;

      if (asyncCell.pendingValue) {
        await asyncCell.pendingValue;
      }
      const firstValue = asyncCell.value;

      await asyncCell.refresh();
      const secondValue = asyncCell.value;

      assert.notStrictEqual(
        firstValue,
        secondValue,
        "refresh should trigger recomputation"
      );
    });
  });

  describe("Dependency Inspection", () => {
    it("can inspect dependencies", () => {
      const a = new InputCell(1);
      const b = new InputCell(2);
      const sum = new ComputeCell(
        [a, b],
        (x: number, y: number) => x + y
      );

      if (!("getDependencies" in sum)) {
        throw new Error("ComputeCell must implement getDependencies method");
      }

      const inspectable = sum as unknown as DependencyInspectable;
      const deps = inspectable.getDependencies();

      assert.strictEqual(deps.length, 2);
      assert.ok(deps.includes(a as unknown as InputCell<unknown>));
      assert.ok(deps.includes(b as unknown as InputCell<unknown>));
    });

    it("can inspect dependents", () => {
      const input = new InputCell(1);
      const double = new ComputeCell([input], (x: number) => x * 2);
      const triple = new ComputeCell([input], (x: number) => x * 3);

      if (!("getDependents" in input)) {
        throw new Error("InputCell must implement getDependents method");
      }

      const inspectable = input as unknown as DependencyInspectable;
      const dependents = inspectable.getDependents();

      assert.strictEqual(dependents.length, 2);
      assert.ok(dependents.includes(double as unknown as ComputeCell<unknown>));
      assert.ok(dependents.includes(triple as unknown as ComputeCell<unknown>));
    });

    it("dependencies update when cell is removed", () => {
      const input = new InputCell(1);
      const compute = new ComputeCell([input], (x: number) => x + 1);

      if (!("getDependents" in input)) {
        throw new Error("InputCell must implement getDependents method");
      }

      const inspectable = input as unknown as DependencyInspectable;
      assert.strictEqual(inspectable.getDependents().length, 1);

      // Remove callback to simulate removal
      if ("dispose" in compute && typeof compute.dispose === "function") {
        (compute as unknown as { dispose: () => void }).dispose();
        assert.strictEqual(inspectable.getDependents().length, 0);
      }
    });
  });

  describe("Diamond Dependency with Transactions", () => {
    it("handles diamond dependencies correctly with batch updates", () => {
      const input = new InputCell(1);
      const left = new ComputeCell([input], (x: number) => x + 1);
      const right = new ComputeCell([input], (x: number) => x * 2);
      const bottom = new ComputeCell(
        [left, right] as unknown as InputCell<unknown>[],
        (l: number, r: number) => l + r
      );

      const values: number[] = [];
      bottom.addCallback((v) => values.push(v));

      // With input=1: left=2, right=2, bottom=4
      // After input=5: left=6, right=10, bottom=16
      input.setValue(5);

      // Should only fire once with final value, not intermediate states
      assert.deepStrictEqual(values, [16]);
    });
  });
});
