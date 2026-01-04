/// Runs [tasks] with at most [concurrency] tasks in-flight at any time.
///
/// - Preserves the result order (results[i] corresponds to tasks[i]).
/// - If any task fails, stops starting new tasks and completes with that error.
Future<List<T>> runWithConcurrency<T>(
  List<Future<T> Function()> tasks,
  int concurrency,
) async {
  throw UnimplementedError();
}
