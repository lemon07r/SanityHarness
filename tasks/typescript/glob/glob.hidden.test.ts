import { describe, it } from "node:test";
import assert from "node:assert";
import { matchGlob } from "./glob.ts";

describe("matchGlob (hidden)", () => {
  it("treats consecutive '*' as a single '*'", () => {
    assert.strictEqual(matchGlob("a**b", "ab"), true);
    assert.strictEqual(matchGlob("a**b", "axxxb"), true);
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
});
