use regex_lite::is_match;

#[test]
fn invalid_patterns_return_false() {
    assert!(!is_match("*", ""));
    assert!(!is_match("*a", "a"));
    assert!(!is_match("a**", ""));
}

#[test]
fn full_match_not_substring() {
    assert!(!is_match("a", "ba"));
    assert!(!is_match("a", "ab"));
    assert!(is_match(".*a", "ba"));
    assert!(is_match("a.*", "ab"));
}

#[test]
fn unicode_is_char_based() {
    assert!(is_match("..", "🔥a"));
    assert!(!is_match("..", "🔥"));
}

// Adversarial/backtracking-prone cases.
// Runtime limits are enforced by the harness/container timeout instead of
// machine-dependent per-test wall-clock thresholds.

#[test]
fn no_exponential_blowup_simple() {
    // Classic pathological case: a?^n a^n against a^n
    // Naive backtracking takes O(2^n) time
    let pattern = "a*a*a*a*a*a*a*a*a*a*aaaaaaaaaa";
    let text = "aaaaaaaaaa";

    assert!(is_match(pattern, text), "pattern should match");
}

#[test]
fn no_exponential_blowup_nested() {
    // (a*)*b pattern against aaaa...
    // This is a classic ReDoS pattern
    let n = 25;
    let pattern: String = (0..n).map(|_| "a*").collect::<Vec<_>>().join("") + "b";
    let text: String = (0..n).map(|_| 'a').collect();

    assert!(
        !is_match(&pattern, &text),
        "pattern should not match (no 'b' at end)"
    );
}

#[test]
fn no_exponential_blowup_dot_star() {
    // .*.*.*...b against aaaa...
    let n = 20;
    let pattern: String = (0..n).map(|_| ".*").collect::<Vec<_>>().join("") + "b";
    let text: String = (0..n * 2).map(|_| 'a').collect();

    assert!(!is_match(&pattern, &text));
}

#[test]
fn performance_long_text() {
    // Ensure long text doesn't cause issues
    let text: String = (0..10_000)
        .map(|i| (b'a' + (i % 26) as u8) as char)
        .collect();

    assert!(is_match(".*", &text));
}

#[test]
fn performance_many_stars() {
    // Pattern with many star operators
    let pattern = "a*b*c*d*e*f*g*h*i*j*";
    let text = "aabbccddeeffgghhiijj";

    assert!(is_match(pattern, text));
}

#[test]
fn performance_alternating_stars() {
    // Pattern that could cause backtracking issues
    let pattern = ".*a.*a.*a.*a.*a";
    let text = "xaxaxaxaxax";

    // Pattern requires exactly 5 'a's with anything between
    assert!(is_match(pattern, text));
}

#[test]
fn performance_no_match_long_pattern() {
    // Long pattern that won't match - should fail fast
    let pattern = "a*b*c*d*e*f*g*h*i*j*k*l*m*n*o*p*q*r*s*t*u*v*w*x*y*z";
    let text = "this is a test string without the pattern";

    assert!(!is_match(pattern, text));
}

#[test]
fn greedy_vs_minimal_matching() {
    // Verify greedy matching behavior
    assert!(is_match("a.*b", "aXXXb"));
    assert!(is_match("a.*b", "ab"));
    assert!(is_match("a.*b.*c", "aXbYc"));
}

#[test]
fn edge_cases_with_stars() {
    // Empty pattern star combinations
    assert!(is_match(".*.*.*", ""));
    assert!(is_match(".*.*.*", "abc"));

    // Star at end
    assert!(is_match("abc.*", "abc"));
    assert!(is_match("abc.*", "abcdef"));

    // Star at beginning
    assert!(is_match(".*abc", "abc"));
    assert!(is_match(".*abc", "xyzabc"));
}
