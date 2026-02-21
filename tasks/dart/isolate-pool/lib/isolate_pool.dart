import 'dart:async';
import 'dart:isolate';

/// A pool of isolates for executing tasks in parallel.
///
/// Behavioral requirements:
/// - `start()` initializes worker isolates and their communication channels.
/// - `submit()` dispatches work to running workers and returns each task result.
/// - Task failures must propagate to the returned future and must not corrupt
///   the pool: subsequent valid submissions should still work.
/// - `shutdown()` waits for pending tasks before stopping workers.
/// - `shutdownNow()` terminates workers immediately.
class IsolatePool {
  /// Creates a new isolate pool with the specified number of workers.
  IsolatePool(int workerCount) {
    throw UnimplementedError('Please implement IsolatePool constructor');
  }

  /// Starts the pool and initializes all worker isolates.
  Future<void> start() {
    throw UnimplementedError('Please implement start');
  }

  /// Submits a task for execution and returns the result.
  /// 
  /// The [task] function will be executed in one of the worker isolates.
  /// The function must be a top-level function or a static method.
  /// If task execution fails, the returned future completes with that error.
  Future<R> submit<T, R>(R Function(T) task, T argument) {
    throw UnimplementedError('Please implement submit');
  }

  /// Shuts down the pool, waiting for all pending tasks to complete.
  Future<void> shutdown() {
    throw UnimplementedError('Please implement shutdown');
  }

  /// Immediately terminates all workers without waiting for pending tasks.
  void shutdownNow() {
    throw UnimplementedError('Please implement shutdownNow');
  }

  /// Returns the number of workers in the pool.
  int get workerCount {
    throw UnimplementedError('Please implement workerCount');
  }

  /// Returns true if the pool has been started.
  bool get isRunning {
    throw UnimplementedError('Please implement isRunning');
  }
}

/// Entry point for worker isolates.
/// This must be a top-level function.
///
/// Communication contract:
/// - Worker and main isolate must establish a two-way messaging channel.
/// - Each submitted job message must include the callable, its argument, and a
///   response channel for either a success value or an error payload.
void _workerEntryPoint(SendPort sendPort) {
  throw UnimplementedError('Please implement worker entry point');
}
