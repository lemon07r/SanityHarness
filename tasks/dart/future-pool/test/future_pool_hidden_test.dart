import 'dart:async';
import 'package:test/test.dart';

import '../lib/future_pool.dart';

void main() {
  group('runWithConcurrency (hidden)', () {
    test('throws on invalid concurrency', () async {
      await expectLater(
        () => runWithConcurrency<int>([], 0),
        throwsArgumentError,
      );
      await expectLater(
        () => runWithConcurrency<int>([], -1),
        throwsArgumentError,
      );
    });

    test('stops starting new tasks after first failure', () async {
      final release1 = Completer<void>();
      final started2 = Completer<void>();
      final failed0 = Completer<void>();

      final tasks = <Future<int> Function()>[
        () async {
          failed0.complete();
          throw StateError('boom');
        },
        () async {
          await release1.future;
          return 1;
        },
        () async {
          started2.complete();
          return 2;
        },
      ];

      final fut = runWithConcurrency(tasks, 2);

      await failed0.future;
      // Give the scheduler a chance to enqueue more work.
      await Future<void>.delayed(Duration.zero);
      expect(started2.isCompleted, isFalse);

      release1.complete();
      await expectLater(fut, throwsA(isA<StateError>()));
      expect(started2.isCompleted, isFalse);
    });

    test('empty task list returns empty results', () async {
      final results = await runWithConcurrency<int>([], 3);
      expect(results, isEmpty);
    });
  });
}
