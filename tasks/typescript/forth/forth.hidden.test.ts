import { describe, it } from "node:test";
import assert from "node:assert";
import { Forth, ValueError } from "./forth.ts";

describe("Forth (hidden)", () => {
  it("errors on unknown word", () => {
    const forth = new Forth();
    assert.throws(() => forth.evaluate("unknown"), ValueError);
  });

  it("errors on invalid word definition (missing terminator)", () => {
    const forth = new Forth();
    assert.throws(() => forth.evaluate(": foo 1"), ValueError);
  });

  it("does not allow defining a number", () => {
    const forth = new Forth();
    assert.throws(() => forth.evaluate(": 1 2 ;"), ValueError);
  });

  it("user-defined words are case-insensitive", () => {
    const forth = new Forth();
    forth.evaluate(": Double 2 * ;");
    forth.evaluate("5 DOUBLE");
    assert.deepStrictEqual(forth.stackValue, [10]);
  });

  it("definitions are expanded at definition time", () => {
    const forth = new Forth();
    forth.evaluate(": foo 5 ;");
    forth.evaluate(": bar foo ;");
    forth.evaluate(": foo 6 ;");
    forth.evaluate("bar");
    assert.deepStrictEqual(forth.stackValue, [5]);
  });

  it("errors on insufficient values for multiplication", () => {
    const forth = new Forth();
    assert.throws(() => forth.evaluate("1 *"), ValueError);
  });

  it("errors on insufficient values for division", () => {
    const forth = new Forth();
    assert.throws(() => forth.evaluate("1 /"), ValueError);
  });
});
