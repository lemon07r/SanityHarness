import { describe, it } from "node:test";
import assert from "node:assert";
import { Readable } from "node:stream";
import { parseCsv } from "./csv.ts";

describe("parseCsv (hidden)", () => {
  it("handles quoted fields spanning chunks and containing newlines", async () => {
    const chunks = ['a,"b\n', 'c",d\n'];
    const rows = await parseCsv(Readable.from(chunks));
    assert.deepStrictEqual(rows, [["a", "b\nc", "d"]]);
  });

  it("treats a trailing delimiter as an empty field", async () => {
    const rows = await parseCsv(Readable.from("a,b,\n"));
    assert.deepStrictEqual(rows, [["a", "b", ""]]);
  });

  it("throws on unterminated quotes", async () => {
    await assert.rejects(
      () => parseCsv(Readable.from('a,"b\n')),
      /quote|unterminated|invalid/i
    );
  });

  it("parses many rows efficiently", async () => {
    const lines: string[] = [];
    for (let i = 0; i < 2000; i++) {
      lines.push(`${i},${i + 1},${i + 2}`);
    }
    const rows = await parseCsv(Readable.from(lines.join("\n") + "\n"));
    assert.strictEqual(rows.length, 2000);
    assert.deepStrictEqual(rows[0], ["0", "1", "2"]);
    assert.deepStrictEqual(rows[1999], ["1999", "2000", "2001"]);
  });
});
