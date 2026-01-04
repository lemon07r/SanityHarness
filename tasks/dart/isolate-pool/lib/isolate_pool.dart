import 'dart:async';
import 'dart:isolate';

/// A pool of isolates for executing tasks in parallel.
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
void _workerEntryPoint(SendPort sendPort) {
  throw UnimplementedError('Please implement worker entry point');
}
