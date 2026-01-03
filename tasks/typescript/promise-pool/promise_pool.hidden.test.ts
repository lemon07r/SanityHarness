import { describe, it } from "node:test";
import assert from "node:assert";
import { promisePool } from "./promise_pool.ts";

function tick(): Promise<void> {
  return new Promise((r) => setImmediate(r));
}

describe("promisePool (hidden)", () => {
  it("treats concurrency <= 0 as 1", async () => {
    const started: number[] = [];
    const tasks = [
      async () => {
        started.push(0);
        return 1;
      },
      async () => {
        started.push(1);
        return 2;
      },
    ];

    const p = promisePool(tasks, 0);
    await tick();
    assert.deepStrictEqual(started, [0]);
    assert.deepStrictEqual(await p, [1, 2]);
  });

  it("rejects and does not start new tasks after the first failure", async () => {
    const started: number[] = [];

    const tasks = [
      async () => {
        started.push(0);
        throw new Error("boom");
      },
      async () => {
        started.push(1);
        return 2;
      },
    ];

    const p = promisePool(tasks, 1);
    await tick();
    assert.deepStrictEqual(started, [0]);

    await assert.rejects(p);
    assert.deepStrictEqual(started, [0]);
  });
});
