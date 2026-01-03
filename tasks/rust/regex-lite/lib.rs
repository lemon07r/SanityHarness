/// Returns true if `text` matches `pattern`.
///
/// Supported syntax:
/// - `.` matches any single character
/// - `*` matches zero or more repetitions of the previous token
///
/// The entire `text` must match the entire `pattern`.
pub fn is_match(pattern: &str, text: &str) -> bool {
    let _ = (pattern, text);
    todo!("Implement is_match")
}
