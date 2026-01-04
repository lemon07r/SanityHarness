import { describe, it } from "node:test";
import assert from "node:assert";
import { Readable } from "node:stream";
import { parseCsv } from "./csv.ts";

describe("parseCsv", () => {
  it("parses basic CSV", async () => {
    const rows = await parseCsv(Readable.from("a,b\nc,d\n"));
    assert.deepStrictEqual(rows, [
      ["a", "b"],
      ["c", "d"],
    ]);
  });

  it("supports quoted fields containing delimiters", async () => {
    const rows = await parseCsv(Readable.from('a,"b,c"\n'));
    assert.deepStrictEqual(rows, [["a", "b,c"]]);
  });

  it("supports escaped quotes inside quoted fields", async () => {
    const rows = await parseCsv(Readable.from('a,"b""c"\n'));
    assert.deepStrictEqual(rows, [["a", 'b"c']]);
  });

  it("accepts CRLF line endings", async () => {
    const rows = await parseCsv(Readable.from("a,b\r\nc,d\r\n"));
    assert.deepStrictEqual(rows, [
      ["a", "b"],
      ["c", "d"],
    ]);
  });

  it("supports a custom delimiter", async () => {
    const rows = await parseCsv(Readable.from("a;b\nc;d\n"), {
      delimiter: ";",
    });
    assert.deepStrictEqual(rows, [
      ["a", "b"],
      ["c", "d"],
    ]);
  });
});
