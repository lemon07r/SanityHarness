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

      await expectLater(Future.sync(() => cache.set('b', 2)), throwsStateError);
      await expectLater(Future.sync(() => cache.get('a')), throwsStateError);
      await expectLater(Future.sync(() => cache.remove('a')), throwsStateError);

      expect(() => cache.watch('a'), throwsStateError);
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
