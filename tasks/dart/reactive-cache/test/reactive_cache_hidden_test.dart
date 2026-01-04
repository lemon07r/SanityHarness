import 'dart:async';
import 'package:test/test.dart';
import '../lib/reactive_cache.dart';

void main() {
  group('ReactiveCache (hidden)', () {
    test('negative default TTL throws', () {
      expect(
        () => ReactiveCache<String, int>(
          defaultTtl: Duration(seconds: -1),
        ),
        throwsArgumentError,
      );
    });

    test('dispose makes cache unusable', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('a', 1);
      await cache.dispose();

      await expectLater(
        Future.sync(() => cache.set('b', 2)),
        throwsStateError,
      );
      await expectLater(
        Future.sync(() => cache.get('a')),
        throwsStateError,
      );
      await expectLater(
        Future.sync(() => cache.remove('a')),
        throwsStateError,
      );
    });

    test('watch emits when value is set later', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      final values = <int>[];
      late final StreamSubscription<int> sub;
      sub = cache.watch('k').listen(values.add);

      await Future.delayed(Duration(milliseconds: 10));
      await cache.set('k', 42);
      await Future.delayed(Duration(milliseconds: 20));

      expect(values, contains(42));

      await sub.cancel();
      await cache.dispose();
    });
  });
}
      final subscription = cache.watch('key').listen(
        (v) {},
        onDone: () => cleared = true,
      );

      await Future.delayed(Duration(milliseconds: 10));
      await cache.clear();
      await Future.delayed(Duration(milliseconds: 10));

      // Stream should be closed or receive null/done signal
      expect(cleared, isTrue);

      await subscription.cancel();
      await cache.dispose();
    });
  });

  // ========== Edge Cases ==========

  group('edge cases', () {
    test('handles concurrent access correctly', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async {
          await Future.delayed(Duration(milliseconds: 10));
          return int.parse(key);
        },
      );

      // Concurrent gets should work correctly
      final futures = List.generate(10, (i) => cache.get('$i'));
      final results = await Future.wait(futures);

      expect(results, equals(List.generate(10, (i) => i)));

      await cache.dispose();
    });

    test('dispose cancels all pending operations', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key', 42);
      final subscription = cache.watch('key').listen((_) {});

      await cache.dispose();

      // Operations after dispose should fail gracefully
      expect(() => cache.set('new', 1), throwsStateError);

      await subscription.cancel();
    });

    test('negative TTL throws', () {
      expect(
        () => ReactiveCache<String, int>(
          defaultTtl: Duration(seconds: -1),
        ),
        throwsArgumentError,
      );
    });
  });
}

/// Statistics about cache performance.
/// This class must be implemented by the cache.
class CacheStats {
  final int hits;
  final int misses;
  final int entryCount;

  CacheStats({
    required this.hits,
    required this.misses,
    required this.entryCount,
  });

  double get hitRate {
    final total = hits + misses;
    return total == 0 ? 0.0 : hits / total;
  }
}
