import 'dart:async';
import 'dart:math' as math;

import 'package:test/test.dart';

import '../lib/future_pool.dart';

void main() {
  group('runWithConcurrency', () {
    test('limits concurrency and preserves result order', () async {
      var running = 0;
      var maxRunning = 0;

      final started = List.generate(4, (_) => Completer<void>());
      final release = List.generate(4, (_) => Completer<void>());

      final tasks = List<Future<int> Function()>.generate(4, (i) {
        return () async {
          running++;
          maxRunning = math.max(maxRunning, running);
          started[i].complete();

          await release[i].future;
          running--;
          return i;
        };
      });

      final resultFuture = runWithConcurrency(tasks, 2);

      await Future.wait([started[0].future, started[1].future]);
      expect(maxRunning, equals(2));
      expect(started[2].isCompleted, isFalse);

      // Complete out-of-order; results must still be in input order.
      release[1].complete();
      await started[2].future;

      release[0].complete();
      await started[3].future;

      release[3].complete();
      release[2].complete();

      final results = await resultFuture;
      expect(results, equals([0, 1, 2, 3]));
      expect(maxRunning, equals(2));
    });
  });
}
