import 'package:test/test.dart';
import '../lib/isolate_pool.dart';

// Test helper functions (must be top-level for isolate execution)
int double(int x) => x * 2;

String throwError(String msg) {
  throw Exception(msg);
}

void main() {
  group('IsolatePool (hidden)', () {
    test(
      'propagates task errors and remains usable',
      () async {
        final pool = IsolatePool(2);
        await pool.start();

        await expectLater(
          pool.submit(throwError, 'boom'),
          throwsA(isA<Exception>()),
        );

        final ok = await pool.submit(double, 5);
        expect(ok, equals(10));

        await pool.shutdown();
      },
      timeout: Timeout(Duration(seconds: 5)),
    );

    test(
      'concurrent submissions are safe',
      () async {
        final pool = IsolatePool(4);
        await pool.start();

        final futures = List.generate(50, (i) => pool.submit(double, i));
        final results = await Future.wait(futures);

        for (var i = 0; i < 50; i++) {
          expect(results[i], equals(i * 2));
        }

        await pool.shutdown();
      },
      timeout: Timeout(Duration(seconds: 10)),
    );
  });
}
