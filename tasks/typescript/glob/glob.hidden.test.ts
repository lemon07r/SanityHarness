import { describe, it } from "node:test";
import assert from "node:assert";
import { matchGlob } from "./glob.ts";

describe("matchGlob (hidden)", () => {
  it("treats consecutive '*' as a single '*'", () => {
    assert.strictEqual(matchGlob("a**b", "ab"), true);
    assert.strictEqual(matchGlob("a**b", "axxxb"), true);
  });

  it("treats a trailing escape as a literal backslash", () => {
    assert.strictEqual(matchGlob("\\", "\\"), true);
  });
});
