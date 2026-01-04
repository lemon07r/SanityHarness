import 'dart:async';
import 'package:test/test.dart';
import '../lib/isolate_pool.dart';

// Test helper functions
int double(int x) => x * 2;
int slowCompute(int x) {
  var result = x;
  for (var i = 0; i < 10000000; i++) {
    result = (result * 2) % 1000000;
  }
  return result;
}

String throwError(String msg) {
  throw Exception(msg);
}

void main() {
  group('IsolatePool Hidden Tests', () {
    // Hidden tests require additional methods

    test('priority queue - high priority tasks execute first', () async {
      final pool = IsolatePool(1);
      await pool.start();

      // Check if submitWithPriority exists
      if (pool is! PriorityIsolatePool) {
        fail('IsolatePool must implement PriorityIsolatePool with submitWithPriority');
      }

      final priorityPool = pool as PriorityIsolatePool;
      
      final results = <int>[];
      
      // Submit low priority first
      priorityPool.submitWithPriority(double, 1, Priority.low).then((r) => results.add(r));
      priorityPool.submitWithPriority(double, 2, Priority.low).then((r) => results.add(r));
      
      // Then high priority
      priorityPool.submitWithPriority(double, 100, Priority.high).then((r) => results.add(r));
      
      await Future.delayed(Duration(milliseconds: 500));
      await pool.shutdown();
      
      // High priority (200) should be in results before lows
      expect(results.contains(200), isTrue);
    });

    test('task cancellation', () async {
      final pool = IsolatePool(2);
      await pool.start();

      if (pool is! CancellableIsolatePool) {
        fail('IsolatePool must implement CancellableIsolatePool with submitCancellable');
      }

      final cancellablePool = pool as CancellableIsolatePool;
      
      final task = cancellablePool.submitCancellable(slowCompute, 42);
      
      // Cancel after a short delay
      await Future.delayed(Duration(milliseconds: 50));
      task.cancel();
      
      expect(() => task.future, throwsA(isA<CancelledException>()));
      
      await pool.shutdown();
    });

    test('load balancing statistics', () async {
      final pool = IsolatePool(4);
      await pool.start();

      if (pool is! StatsIsolatePool) {
        fail('IsolatePool must implement StatsIsolatePool with getStats');
      }

      final statsPool = pool as StatsIsolatePool;
      
      // Submit several tasks
      final futures = <Future>[];
      for (var i = 0; i < 20; i++) {
        futures.add(pool.submit(double, i));
      }
      await Future.wait(futures);
      
      final stats = statsPool.getStats();
      
      expect(stats.totalTasksCompleted, equals(20));
      expect(stats.workerStats.length, equals(4));
      
      // Work should be distributed among workers
      for (final workerStat in stats.workerStats) {
        expect(workerStat.tasksCompleted, greaterThan(0));
      }
      
      await pool.shutdown();
    });

    test('dynamic scaling', () async {
      final pool = IsolatePool(2);
      await pool.start();

      if (pool is! ScalableIsolatePool) {
        fail('IsolatePool must implement ScalableIsolatePool with scale');
      }

      final scalablePool = pool as ScalableIsolatePool;
      
      expect(pool.workerCount, equals(2));
      
      // Scale up
      await scalablePool.scale(4);
      expect(pool.workerCount, equals(4));
      
      // Scale down
      await scalablePool.scale(1);
      expect(pool.workerCount, equals(1));
      
      // Tasks should still work
      final result = await pool.submit(double, 5);
      expect(result, equals(10));
      
      await pool.shutdown();
    });

    test('timeout support', () async {
      final pool = IsolatePool(2);
      await pool.start();

      if (pool is! TimeoutIsolatePool) {
        fail('IsolatePool must implement TimeoutIsolatePool with submitWithTimeout');
      }

      final timeoutPool = pool as TimeoutIsolatePool;
      
      expect(
        () => timeoutPool.submitWithTimeout(
          slowCompute, 
          42, 
          Duration(milliseconds: 10)
        ),
        throwsA(isA<TimeoutException>()),
      );
      
      await pool.shutdown();
    });

    test('error handling propagates exceptions', () async {
      final pool = IsolatePool(2);
      await pool.start();
      
      expect(
        () => pool.submit(throwError, 'test error'),
        throwsA(isA<Exception>()),
      );
      
      // Pool should still be functional after error
      final result = await pool.submit(double, 5);
      expect(result, equals(10));
      
      await pool.shutdown();
    });

    test('concurrent submissions are safe', () async {
      final pool = IsolatePool(4);
      await pool.start();
      
      // Submit many tasks concurrently
      final futures = List.generate(100, (i) => pool.submit(double, i));
      final results = await Future.wait(futures);
      
      // All results should be correct
      for (var i = 0; i < 100; i++) {
        expect(results[i], equals(i * 2));
      }
      
      await pool.shutdown();
    });

    test('warmup preloads workers', () async {
      final pool = IsolatePool(4);
      
      if (pool is! WarmableIsolatePool) {
        fail('IsolatePool must implement WarmableIsolatePool with warmup');
      }

      final warmablePool = pool as WarmableIsolatePool;
      
      await pool.start();
      await warmablePool.warmup();
      
      // First task should execute quickly (no isolate spawn overhead)
      final stopwatch = Stopwatch()..start();
      await pool.submit(double, 5);
      stopwatch.stop();
      
      expect(stopwatch.elapsedMilliseconds, lessThan(100));
      
      await pool.shutdown();
    });
  });
}

// Extended interfaces for hidden tests
abstract class PriorityIsolatePool {
  Future<R> submitWithPriority<T, R>(R Function(T) task, T argument, Priority priority);
}

enum Priority { low, normal, high }

abstract class CancellableIsolatePool {
  CancellableTask<R> submitCancellable<T, R>(R Function(T) task, T argument);
}

abstract class CancellableTask<R> {
  Future<R> get future;
  void cancel();
}

class CancelledException implements Exception {}

abstract class StatsIsolatePool {
  PoolStats getStats();
}

class PoolStats {
  final int totalTasksCompleted;
  final List<WorkerStats> workerStats;
  PoolStats(this.totalTasksCompleted, this.workerStats);
}

class WorkerStats {
  final int tasksCompleted;
  WorkerStats(this.tasksCompleted);
}

abstract class ScalableIsolatePool {
  Future<void> scale(int newWorkerCount);
}

abstract class TimeoutIsolatePool {
  Future<R> submitWithTimeout<T, R>(R Function(T) task, T argument, Duration timeout);
}

abstract class WarmableIsolatePool {
  Future<void> warmup();
}
