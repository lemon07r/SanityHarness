import type { Readable } from "node:stream";

export type CsvOptions = {
  delimiter?: string;
};

export async function parseCsv(
  input: Readable,
  opts: CsvOptions = {}
): Promise<string[][]> {
  void input;
  void opts;
  throw new Error("Please implement parseCsv");
}
