/**
 * Runs promise-returning tasks with bounded concurrency.
 *
 * Contract:
 * - Resolves results in input order: `out[i]` corresponds to `tasks[i]`.
 * - If `tasks` is empty, resolves to `[]`.
 * - If `concurrency <= 0`, it must be treated as `1`.
 * - At most `concurrency` tasks may be in-flight at any point.
 * - On the first task rejection, the returned promise rejects with that error
 *   and no new tasks may be started. Tasks already started may still settle.
 */
export async function promisePool<T>(
  tasks: Array<() => Promise<T>>,
  concurrency: number
): Promise<T[]> {
  throw new Error("Please implement promisePool");
}
