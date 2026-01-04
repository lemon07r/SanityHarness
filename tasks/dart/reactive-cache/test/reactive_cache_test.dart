import 'dart:async';
import 'package:test/test.dart';
import '../lib/reactive_cache.dart';

void main() {
  group('ReactiveCache', () {
    test('set and get a value', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key1', 42);
      final value = await cache.get('key1');

      expect(value, equals(42));

      await cache.dispose();
    });

    test('get with loader loads value when not in cache', () async {
      final cache = ReactiveCache<String, String>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => 'loaded:$key',
      );

      final value = await cache.get('mykey');

      expect(value, equals('loaded:mykey'));

      await cache.dispose();
    });

    test('get without loader throws when key not present', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      expect(() => cache.get('missing'), throwsStateError);

      await cache.dispose();
    });

    test('setWithTtl respects custom TTL', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.setWithTtl('key1', 100, Duration(milliseconds: 50));

      expect(cache.containsKey('key1'), isTrue);

      // Wait for TTL to expire
      await Future.delayed(Duration(milliseconds: 100));

      expect(cache.containsKey('key1'), isFalse);

      await cache.dispose();
    });

    test('remove removes a key', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key1', 42);
      expect(cache.containsKey('key1'), isTrue);

      final removed = await cache.remove('key1');
      expect(removed, isTrue);
      expect(cache.containsKey('key1'), isFalse);

      await cache.dispose();
    });

    test('remove returns false for non-existent key', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      final removed = await cache.remove('nonexistent');
      expect(removed, isFalse);

      await cache.dispose();
    });

    test('containsKey returns false for expired entries', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(milliseconds: 50),
      );

      await cache.set('key1', 42);
      expect(cache.containsKey('key1'), isTrue);

      await Future.delayed(Duration(milliseconds: 100));

      expect(cache.containsKey('key1'), isFalse);

      await cache.dispose();
    });

    test('watch emits current value and updates', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key1', 1);

      final values = <int>[];
      final subscription = cache.watch('key1').listen((v) => values.add(v));

      // Give time for initial value
      await Future.delayed(Duration(milliseconds: 10));

      await cache.set('key1', 2);
      await cache.set('key1', 3);

      await Future.delayed(Duration(milliseconds: 10));

      expect(values, containsAllInOrder([1, 2, 3]));

      await subscription.cancel();
      await cache.dispose();
    });

    test('watch loads value if not present and loader available', () async {
      final cache = ReactiveCache<String, String>(
        defaultTtl: Duration(seconds: 10),
        loader: (key) async => 'loaded:$key',
      );

      final values = <String>[];
      final subscription = cache.watch('newkey').listen((v) => values.add(v));

      await Future.delayed(Duration(milliseconds: 50));

      expect(values, contains('loaded:newkey'));

      await subscription.cancel();
      await cache.dispose();
    });

    test('expired entries are reloaded on get', () async {
      var loadCount = 0;
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(milliseconds: 50),
        loader: (key) async {
          loadCount++;
          return loadCount;
        },
      );

      final first = await cache.get('key');
      expect(first, equals(1));
      expect(loadCount, equals(1));

      // Wait for expiration
      await Future.delayed(Duration(milliseconds: 100));

      final second = await cache.get('key');
      expect(second, equals(2));
      expect(loadCount, equals(2));

      await cache.dispose();
    });

    test('multiple watchers receive updates', () async {
      final cache = ReactiveCache<String, int>(
        defaultTtl: Duration(seconds: 10),
      );

      await cache.set('key1', 0);

      final values1 = <int>[];
      final values2 = <int>[];

      final sub1 = cache.watch('key1').listen((v) => values1.add(v));
      final sub2 = cache.watch('key1').listen((v) => values2.add(v));

      await Future.delayed(Duration(milliseconds: 10));

      await cache.set('key1', 1);

      await Future.delayed(Duration(milliseconds: 10));

      expect(values1, containsAllInOrder([0, 1]));
      expect(values2, containsAllInOrder([0, 1]));

      await sub1.cancel();
      await sub2.cancel();
      await cache.dispose();
    });
  });

  group('CacheEntry', () {
    test('isExpired returns true after TTL', () async {
      final entry = CacheEntry(
        value: 'test',
        createdAt: DateTime.now(),
        ttl: Duration(milliseconds: 50),
      );

      expect(entry.isExpired, isFalse);

      await Future.delayed(Duration(milliseconds: 100));

      expect(entry.isExpired, isTrue);
    });

    test('remainingTtl decreases over time', () async {
      final entry = CacheEntry(
        value: 'test',
        createdAt: DateTime.now(),
        ttl: Duration(milliseconds: 100),
      );

      final initial = entry.remainingTtl;
      expect(initial.inMilliseconds, greaterThan(50));

      await Future.delayed(Duration(milliseconds: 60));

      final remaining = entry.remainingTtl;
      expect(remaining.inMilliseconds, lessThan(50));
    });
  });
}
