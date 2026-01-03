use std::collections::HashMap;

/// Count the frequency of letters in the given texts using multiple workers.
///
/// Only Unicode letters should be counted (use char::is_alphabetic).
/// Letter case should be ignored (convert to lowercase).
///
/// The `worker_count` parameter indicates how many threads should be used.
pub fn frequency(input: &[&str], worker_count: usize) -> HashMap<char, usize> {
    todo!(
        "Count letter frequency in {:?} using {} workers",
        input,
        worker_count
    )
}
