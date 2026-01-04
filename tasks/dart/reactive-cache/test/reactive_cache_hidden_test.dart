import 'dart:async';
import 'package:test/test.dart';
import '../lib/reactive_cache.dart';

void main() {
  // ========== getStats() Hidden Tests ==========
  // Hidden requirement: getStats() returns CacheStats with hit/miss counts

  group('getStats', () {
    test('tracks cache hits', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key1', 42);
      await cache.get('key1');
      await cache.get('key1');
      await cache.get('key1');

      final stats = cache.getStats();
      expect(stats.hits, equals(3));

      await cache.dispose();
    });

    test('tracks cache misses', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => 0,
      );

      await cache.get('miss1');
      await cache.get('miss2');
      await cache.get('miss3');

      final stats = cache.getStats();
      expect(stats.misses, equals(3));

      await cache.dispose();
    });

    test('tracks entry count', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3);

      final stats = cache.getStats();
      expect(stats.entryCount, equals(3));

      await cache.dispose();
    });

    test('calculates hit rate correctly', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => 0,
      );

      await cache.set('key', 42);
      // 2 hits
      await cache.get('key');
      await cache.get('key');
      // 2 misses
      await cache.get('other1');
      await cache.get('other2');

      final stats = cache.getStats();
      expect(stats.hitRate, closeTo(0.5, 0.01));

      await cache.dispose();
    });
  });

  // ========== setMaxSize() Hidden Tests ==========
  // Hidden requirement: setMaxSize(n) limits cache to n entries with LRU eviction

  group('setMaxSize', () {
    test('evicts oldest entries when max size exceeded', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );
      cache.setMaxSize(3);

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3);
      await cache.set('d', 4); // Should evict 'a'

      expect(cache.containsKey('a'), isFalse);
      expect(cache.containsKey('b'), isTrue);
      expect(cache.containsKey('c'), isTrue);
      expect(cache.containsKey('d'), isTrue);

      await cache.dispose();
    });

    test('uses LRU eviction policy', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );
      cache.setMaxSize(3);

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3);

      // Access 'a' to make it recently used
      await cache.get('a');

      await cache.set('d', 4); // Should evict 'b' (least recently used)

      expect(cache.containsKey('a'), isTrue);
      expect(cache.containsKey('b'), isFalse);
      expect(cache.containsKey('c'), isTrue);
      expect(cache.containsKey('d'), isTrue);

      await cache.dispose();
    });

    test('setMaxSize with zero disables limit', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );
      cache.setMaxSize(2);

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3); // Evicts 'a'

      expect(cache.containsKey('a'), isFalse);

      cache.setMaxSize(0); // Disable limit

      await cache.set('d', 4);
      await cache.set('e', 5);
      await cache.set('f', 6);

      // All should be present now
      expect(cache.containsKey('b'), isTrue);
      expect(cache.containsKey('c'), isTrue);
      expect(cache.containsKey('d'), isTrue);
      expect(cache.containsKey('e'), isTrue);
      expect(cache.containsKey('f'), isTrue);

      await cache.dispose();
    });
  });

  // ========== refresh() Hidden Tests ==========
  // Hidden requirement: refresh(key) reloads a key using the loader

  group('refresh', () {
    test('refresh reloads value using loader', () async {
      var counter = 0;
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => ++counter,
      );

      final first = await cache.get('key');
      expect(first, equals(1));

      await cache.refresh('key');

      final second = await cache.get('key');
      expect(second, equals(2));

      await cache.dispose();
    });

    test('refresh updates TTL', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(milliseconds: 100),
        loader: (key) async => 42,
      );

      await cache.get('key');

      // Wait most of the TTL
      await Future.delayed(Duration(milliseconds: 80));

      // Refresh should reset TTL
      await cache.refresh('key');

      // Wait past original expiry
      await Future.delayed(Duration(milliseconds: 50));

      // Should still be valid
      expect(cache.containsKey('key'), isTrue);

      await cache.dispose();
    });

    test('refresh notifies watchers', () async {
      var counter = 0;
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => ++counter,
      );

      await cache.get('key');

      final values = <int>[];
      final subscription = cache.watch('key').listen((v) => values.add(v));

      await Future.delayed(Duration(milliseconds: 10));

      await cache.refresh('key');

      await Future.delayed(Duration(milliseconds: 10));

      expect(values, contains(1));
      expect(values, contains(2));

      await subscription.cancel();
      await cache.dispose();
    });

    test('refresh throws if no loader and key not present', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      expect(() => cache.refresh('missing'), throwsStateError);

      await cache.dispose();
    });
  });

  // ========== watchAll() Hidden Tests ==========
  // Hidden requirement: watchAll() streams all cache updates

  group('watchAll', () {
    test('watchAll receives all updates', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      final updates = <MapEntry<String, int>>[];
      final subscription = cache.watchAll().listen((e) => updates.add(e));

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3);

      await Future.delayed(Duration(milliseconds: 20));

      expect(updates.length, equals(3));
      expect(updates.map((e) => e.key).toSet(), equals({'a', 'b', 'c'}));
      expect(updates.map((e) => e.value).toSet(), equals({1, 2, 3}));

      await subscription.cancel();
      await cache.dispose();
    });

    test('watchAll receives updates from multiple keys', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      final updates = <MapEntry<String, int>>[];
      final subscription = cache.watchAll().listen((e) => updates.add(e));

      await cache.set('key', 1);
      await cache.set('key', 2);
      await cache.set('other', 100);

      await Future.delayed(Duration(milliseconds: 20));

      expect(updates.length, equals(3));

      await subscription.cancel();
      await cache.dispose();
    });
  });

  // ========== clear() Hidden Tests ==========
  // Hidden requirement: clear() removes all entries and resets stats

  group('clear', () {
    test('clear removes all entries', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('a', 1);
      await cache.set('b', 2);
      await cache.set('c', 3);

      expect(cache.getStats().entryCount, equals(3));

      await cache.clear();

      expect(cache.containsKey('a'), isFalse);
      expect(cache.containsKey('b'), isFalse);
      expect(cache.containsKey('c'), isFalse);
      expect(cache.getStats().entryCount, equals(0));

      await cache.dispose();
    });

    test('clear resets statistics', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => 0,
      );

      await cache.set('key', 42);
      await cache.get('key'); // hit
      await cache.get('miss'); // miss

      var stats = cache.getStats();
      expect(stats.hits, equals(1));
      expect(stats.misses, equals(1));

      await cache.clear();

      stats = cache.getStats();
      expect(stats.hits, equals(0));
      expect(stats.misses, equals(0));

      await cache.dispose();
    });

    test('clear notifies watchers of removal', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key', 42);

      var cleared = false;
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
