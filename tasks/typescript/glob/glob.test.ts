import { describe, it } from "node:test";
import assert from "node:assert";
import { matchGlob } from "./glob.ts";

describe("matchGlob", () => {
  it("matches exact strings", () => {
    assert.strictEqual(matchGlob("abc", "abc"), true);
    assert.strictEqual(matchGlob("abc", "ab"), false);
  });

  it("supports '*' wildcard", () => {
    assert.strictEqual(matchGlob("a*c", "ac"), true);
    assert.strictEqual(matchGlob("a*c", "abbbc"), true);
    assert.strictEqual(matchGlob("*", "anything"), true);
    assert.strictEqual(matchGlob("a*", "a"), true);
    assert.strictEqual(matchGlob("a*", "ab"), true);
  });

  it("supports '?' wildcard", () => {
    assert.strictEqual(matchGlob("a?c", "abc"), true);
    assert.strictEqual(matchGlob("a?c", "ac"), false);
    assert.strictEqual(matchGlob("?", "a"), true);
    assert.strictEqual(matchGlob("?", ""), false);
  });

  it("supports escaping '*' and '?'", () => {
    assert.strictEqual(matchGlob("\\*", "*"), true);
    assert.strictEqual(matchGlob("\\?", "?"), true);
    assert.strictEqual(matchGlob("a\\*b", "a*b"), true);
    assert.strictEqual(matchGlob("a\\?b", "a?b"), true);
  });

  it("escape treats the next character literally", () => {
    assert.strictEqual(matchGlob("\\\\", "\\"), true);
    assert.strictEqual(matchGlob("\\\\a", "\\a"), true);
  });

  it("handles empty pattern and input", () => {
    assert.strictEqual(matchGlob("", ""), true);
    assert.strictEqual(matchGlob("", "a"), false);
  });
});
