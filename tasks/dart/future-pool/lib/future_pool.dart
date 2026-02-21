/// Runs [tasks] with at most [concurrency] tasks in-flight at any time.
///
/// Contract:
/// - Preserves input order (`results[i]` corresponds to `tasks[i]`).
/// - Throws [ArgumentError] if [concurrency] is less than 1.
/// - If [tasks] is empty, returns an empty list.
/// - If any started task fails, completes with that error and stops starting
///   new tasks. Tasks that were already started may still complete.
Future<List<T>> runWithConcurrency<T>(
  List<Future<T> Function()> tasks,
  int concurrency,
) async {
  throw UnimplementedError();
}
