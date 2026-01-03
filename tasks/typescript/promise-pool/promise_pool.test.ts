import { describe, it } from "node:test";
import assert from "node:assert";
import { promisePool } from "./promise_pool.ts";

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (v: T) => void;
  reject: (e: unknown) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function tick(): Promise<void> {
  return new Promise((r) => setImmediate(r));
}

describe("promisePool", () => {
  it("resolves empty input", async () => {
    const out = await promisePool([], 3);
    assert.deepStrictEqual(out, []);
  });

  it("respects concurrency and preserves order", async () => {
    const started: number[] = [];
    const d0 = deferred<number>();
    const d1 = deferred<number>();
    const d2 = deferred<number>();

    const tasks = [
      () => {
        started.push(0);
        return d0.promise;
      },
      () => {
        started.push(1);
        return d1.promise;
      },
      () => {
        started.push(2);
        return d2.promise;
      },
    ];

    const p = promisePool(tasks, 2);
    await tick();

    assert.deepStrictEqual(started, [0, 1]);

    d1.resolve(10);
    await tick();
    assert.deepStrictEqual(started, [0, 1, 2]);

    d0.resolve(5);
    d2.resolve(15);

    const out = await p;
    assert.deepStrictEqual(out, [5, 10, 15]);
  });
});
