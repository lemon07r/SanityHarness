import { describe, it } from "node:test";
import assert from "node:assert";
import { InputCell, ComputeCell } from "./react.ts";

describe("React (hidden)", () => {
  it("supports multiple callbacks", () => {
    const input = new InputCell(1);
    const output = new ComputeCell([input], (x: number) => x + 1);

    const a: number[] = [];
    const b: number[] = [];

    output.addCallback((v) => a.push(v));
    output.addCallback((v) => b.push(v));

    input.setValue(2);

    assert.deepStrictEqual(a, [3]);
    assert.deepStrictEqual(b, [3]);
  });

  it("removing one callback does not remove others", () => {
    const input = new InputCell(1);
    const output = new ComputeCell([input], (x: number) => x + 1);

    const a: number[] = [];
    const b: number[] = [];

    const cbA = (v: number) => a.push(v);
    const cbB = (v: number) => b.push(v);

    output.addCallback(cbA);
    output.addCallback(cbB);

    output.removeCallback(cbA);
    input.setValue(5);

    assert.deepStrictEqual(a, []);
    assert.deepStrictEqual(b, [6]);
  });
});
