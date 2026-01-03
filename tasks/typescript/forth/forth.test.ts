import { describe, it } from "node:test";
import assert from "node:assert";
import { Forth, ValueError } from "./forth.ts";

describe("Forth", () => {
  it("numbers are pushed onto the stack", () => {
    const forth = new Forth();
    forth.evaluate("1 2 3 4 5");
    assert.deepStrictEqual(forth.stackValue, [1, 2, 3, 4, 5]);
  });

  it("pushes negative numbers", () => {
    const forth = new Forth();
    forth.evaluate("-1 -2 -3");
    assert.deepStrictEqual(forth.stackValue, [-1, -2, -3]);
  });

  describe("addition", () => {
    it("adds two numbers", () => {
      const forth = new Forth();
      forth.evaluate("1 2 +");
      assert.deepStrictEqual(forth.stackValue, [3]);
    });

    it("errors with insufficient values", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("1 +"), ValueError);
    });
  });

  describe("subtraction", () => {
    it("subtracts two numbers", () => {
      const forth = new Forth();
      forth.evaluate("3 4 -");
      assert.deepStrictEqual(forth.stackValue, [-1]);
    });

    it("errors with insufficient values", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("1 -"), ValueError);
    });
  });

  describe("multiplication", () => {
    it("multiplies two numbers", () => {
      const forth = new Forth();
      forth.evaluate("2 4 *");
      assert.deepStrictEqual(forth.stackValue, [8]);
    });
  });

  describe("division", () => {
    it("divides two numbers", () => {
      const forth = new Forth();
      forth.evaluate("12 3 /");
      assert.deepStrictEqual(forth.stackValue, [4]);
    });

    it("performs integer division", () => {
      const forth = new Forth();
      forth.evaluate("8 3 /");
      assert.deepStrictEqual(forth.stackValue, [2]);
    });

    it("errors on division by zero", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("4 0 /"), ValueError);
    });
  });

  describe("dup", () => {
    it("duplicates the top value", () => {
      const forth = new Forth();
      forth.evaluate("1 dup");
      assert.deepStrictEqual(forth.stackValue, [1, 1]);
    });

    it("errors with empty stack", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("dup"), ValueError);
    });
  });

  describe("drop", () => {
    it("removes the top value", () => {
      const forth = new Forth();
      forth.evaluate("1 2 drop");
      assert.deepStrictEqual(forth.stackValue, [1]);
    });

    it("errors with empty stack", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("drop"), ValueError);
    });
  });

  describe("swap", () => {
    it("swaps the top two values", () => {
      const forth = new Forth();
      forth.evaluate("1 2 swap");
      assert.deepStrictEqual(forth.stackValue, [2, 1]);
    });

    it("errors with insufficient values", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("1 swap"), ValueError);
    });
  });

  describe("over", () => {
    it("copies the second value", () => {
      const forth = new Forth();
      forth.evaluate("1 2 over");
      assert.deepStrictEqual(forth.stackValue, [1, 2, 1]);
    });

    it("errors with insufficient values", () => {
      const forth = new Forth();
      assert.throws(() => forth.evaluate("1 over"), ValueError);
    });
  });

  describe("user-defined words", () => {
    it("can define and use a word", () => {
      const forth = new Forth();
      forth.evaluate(": double 2 * ;");
      forth.evaluate("5 double");
      assert.deepStrictEqual(forth.stackValue, [10]);
    });

    it("can use words within words", () => {
      const forth = new Forth();
      forth.evaluate(": double 2 * ;");
      forth.evaluate(": quad double double ;");
      forth.evaluate("3 quad");
      assert.deepStrictEqual(forth.stackValue, [12]);
    });
  });

  describe("case insensitivity", () => {
    it("DUP is case-insensitive", () => {
      const forth = new Forth();
      forth.evaluate("1 DUP Dup dup");
      assert.deepStrictEqual(forth.stackValue, [1, 1, 1, 1]);
    });
  });
});
