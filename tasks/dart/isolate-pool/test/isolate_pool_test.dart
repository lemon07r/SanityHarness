import 'package:test/test.dart';
import '../lib/isolate_pool.dart';

// Test helper functions (must be top-level for isolate execution)
int double(int x) => x * 2;
int add10(int x) => x + 10;
int slowDouble(int x) {
  // Simulate slow operation
  var result = 0;
  for (var i = 0; i < 1000000; i++) {
    result = x * 2;
  }
  return result;
}

void main() {
  group('IsolatePool', () {
    test('can be created with worker count', () {
      final pool = IsolatePool(4);
      expect(pool.workerCount, equals(4));
    });

    test('starts and stops', () async {
      final pool = IsolatePool(2);
      expect(pool.isRunning, isFalse);
      
      await pool.start();
      expect(pool.isRunning, isTrue);
      
      await pool.shutdown();
      expect(pool.isRunning, isFalse);
    });

    test('executes simple task', () async {
      final pool = IsolatePool(2);
      await pool.start();
      
      final result = await pool.submit(double, 5);
      expect(result, equals(10));
      
      await pool.shutdown();
    });

    test('executes multiple tasks', () async {
      final pool = IsolatePool(2);
      await pool.start();
      
      final futures = <Future<int>>[];
      for (var i = 0; i < 10; i++) {
        futures.add(pool.submit(double, i));
      }
      
      final results = await Future.wait(futures);
      expect(results, equals([0, 2, 4, 6, 8, 10, 12, 14, 16, 18]));
      
      await pool.shutdown();
    });

    test('different tasks can be submitted', () async {
      final pool = IsolatePool(2);
      await pool.start();
      
      final result1 = await pool.submit(double, 5);
      final result2 = await pool.submit(add10, 5);
      
      expect(result1, equals(10));
      expect(result2, equals(15));
      
      await pool.shutdown();
    });

    test('shutdown waits for pending tasks', () async {
      final pool = IsolatePool(1);
      await pool.start();
      
      final future = pool.submit(slowDouble, 5);
      
      // Start shutdown while task is running
      final shutdownFuture = pool.shutdown();
      
      // Both should complete
      final result = await future;
      await shutdownFuture;
      
      expect(result, equals(10));
    });

    test('shutdownNow terminates immediately', () async {
      final pool = IsolatePool(2);
      await pool.start();
      
      // Submit a slow task
      pool.submit(slowDouble, 5);
      
      // Immediate shutdown
      pool.shutdownNow();
      
      expect(pool.isRunning, isFalse);
    });
  });
}
