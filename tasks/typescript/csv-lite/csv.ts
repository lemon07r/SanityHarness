import type { Readable } from "node:stream";

export type CsvOptions = {
  delimiter?: string;
};

/**
 * Parses CSV rows from a stream.
 *
 * Contract:
 * - Default delimiter is `,` unless `opts.delimiter` is provided.
 * - Supports quoted fields, including delimiters and newlines inside quotes.
 * - Uses doubled quotes (`""`) to represent a literal quote inside a quoted
 *   field.
 * - Accepts both LF and CRLF row endings.
 * - A trailing delimiter creates an empty final field.
 * - Rejects malformed CSV input (for example, unterminated quoted fields).
 */
export async function parseCsv(
  input: Readable,
  opts: CsvOptions = {}
): Promise<string[][]> {
  void input;
  void opts;
  throw new Error("Please implement parseCsv");
}
