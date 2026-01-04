use parallel_letter_frequency::frequency;
use std::collections::HashMap;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

/// Verifies that the implementation actually uses the specified number of workers.
#[test]
fn uses_multiple_threads() {
    // Create texts that take measurable time to process
    let texts: Vec<String> = (0..10)
        .map(|i| {
            (0..100_000)
                .map(|j| (b'a' + ((i + j) % 26) as u8) as char)
                .collect()
        })
        .collect();
    let text_refs: Vec<&str> = texts.iter().map(|s| s.as_str()).collect();

    // Time with 1 worker
    let start = Instant::now();
    let _ = frequency(&text_refs, 1);
    let single_duration = start.elapsed();

    // Time with 4 workers
    let start = Instant::now();
    let _ = frequency(&text_refs, 4);
    let multi_duration = start.elapsed();

    // With sufficient work, 4 workers should be faster than 1
    // Allow for some overhead, require at least 1.3x speedup
    let speedup = single_duration.as_secs_f64() / multi_duration.as_secs_f64();
    assert!(
        speedup > 1.3 || single_duration < Duration::from_millis(50),
        "Expected parallel speedup with 4 workers, got {:.2}x (single: {:?}, multi: {:?})",
        speedup,
        single_duration,
        multi_duration
    );
}

/// Verifies that worker_count of 0 is handled gracefully.
#[test]
fn zero_workers_handled() {
    let texts = ["abc", "def"];
    // Should either panic with a clear message or use at least 1 worker
    let result = std::panic::catch_unwind(|| frequency(&texts, 0));

    match result {
        Ok(map) => {
            // If it doesn't panic, it should still produce correct results
            assert_eq!(map.len(), 6);
        }
        Err(_) => {
            // Panicking is acceptable for invalid input
        }
    }
}

/// Verifies that more workers than texts is handled correctly.
#[test]
fn more_workers_than_texts() {
    let texts = ["ab", "cd"];
    let result = frequency(&texts, 100);

    assert_eq!(result.get(&'a'), Some(&1));
    assert_eq!(result.get(&'b'), Some(&1));
    assert_eq!(result.get(&'c'), Some(&1));
    assert_eq!(result.get(&'d'), Some(&1));
}

/// Verifies thread-safety with concurrent access.
#[test]
fn thread_safety() {
    let texts: Vec<String> = (0..100).map(|i| format!("text{}", i)).collect();
    let text_refs: Vec<&str> = texts.iter().map(|s| s.as_str()).collect();

    // Run multiple frequency calls concurrently
    let handles: Vec<_> = (0..4)
        .map(|_| {
            let texts_clone = text_refs.clone();
            std::thread::spawn(move || frequency(&texts_clone, 4))
        })
        .collect();

    let results: Vec<_> = handles.into_iter().map(|h| h.join().unwrap()).collect();

    // All results should be identical
    for result in &results[1..] {
        assert_eq!(results[0], *result);
    }
}

/// Verifies performance scales with worker count.
#[test]
fn performance_scaling() {
    // Skip on single-core systems
    if std::thread::available_parallelism()
        .map(|p| p.get())
        .unwrap_or(1)
        < 2
    {
        return;
    }

    let texts: Vec<String> = (0..20)
        .map(|i| {
            (0..50_000)
                .map(|j| (b'a' + ((i * 7 + j * 3) % 26) as u8) as char)
                .collect()
        })
        .collect();
    let text_refs: Vec<&str> = texts.iter().map(|s| s.as_str()).collect();

    let mut durations = Vec::new();

    for workers in [1, 2, 4] {
        let start = Instant::now();
        let _ = frequency(&text_refs, workers);
        durations.push((workers, start.elapsed()));
    }

    // Verify that more workers generally means faster execution
    // (with some tolerance for overhead)
    let (_, dur_1) = durations[0];
    let (_, dur_2) = durations[1];
    let (_, dur_4) = durations[2];

    // 2 workers should be faster than 1 (allow 10% margin)
    if dur_1 > Duration::from_millis(100) {
        assert!(
            dur_2 < dur_1 + Duration::from_millis(dur_1.as_millis() as u64 / 10),
            "2 workers ({:?}) should be faster than 1 worker ({:?})",
            dur_2,
            dur_1
        );
    }
}

/// Verifies correct handling of very large inputs.
#[test]
fn large_input_handling() {
    let large_text: String = (0..1_000_000)
        .map(|i| (b'a' + (i % 26) as u8) as char)
        .collect();

    let texts = [large_text.as_str()];
    let result = frequency(&texts, 4);

    // Each letter should appear approximately 1_000_000 / 26 times
    let expected_per_letter = 1_000_000 / 26;
    for c in 'a'..='z' {
        let count = *result.get(&c).unwrap_or(&0);
        assert!(
            count >= expected_per_letter - 1 && count <= expected_per_letter + 1,
            "Letter {} count {} not in expected range",
            c,
            count
        );
    }
}

/// Verifies empty text handling with multiple workers.
#[test]
fn empty_texts_with_workers() {
    let texts = ["", "", ""];
    let result = frequency(&texts, 4);
    assert!(result.is_empty());
}

/// Verifies mixed empty and non-empty texts.
#[test]
fn mixed_empty_and_nonempty() {
    let texts = ["", "abc", "", "def", ""];
    let result = frequency(&texts, 3);

    assert_eq!(result.get(&'a'), Some(&1));
    assert_eq!(result.get(&'b'), Some(&1));
    assert_eq!(result.get(&'c'), Some(&1));
    assert_eq!(result.get(&'d'), Some(&1));
    assert_eq!(result.get(&'e'), Some(&1));
    assert_eq!(result.get(&'f'), Some(&1));
}
