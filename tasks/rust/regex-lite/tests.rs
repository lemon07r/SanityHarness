use regex_lite::is_match;

#[test]
fn empty_pattern() {
    assert!(is_match("", ""));
    assert!(!is_match("", "a"));
}

#[test]
fn literal_match() {
    assert!(is_match("abc", "abc"));
    assert!(!is_match("abc", "ab"));
    assert!(!is_match("abc", "abcd"));
}

#[test]
fn dot_matches_any_single_char() {
    assert!(is_match(".", "x"));
    assert!(!is_match(".", ""));
    assert!(!is_match(".", "xy"));
}

#[test]
fn star_matches_zero_or_more() {
    assert!(is_match("a*", ""));
    assert!(is_match("a*", "a"));
    assert!(is_match("a*", "aaaa"));
    assert!(!is_match("a*", "b"));
}

#[test]
fn mixed_tokens() {
    assert!(is_match("ab*c", "ac"));
    assert!(is_match("ab*c", "abc"));
    assert!(is_match("ab*c", "abbbc"));
    assert!(!is_match("ab*c", "abbd"));
}

#[test]
fn dot_star_matches_anything() {
    assert!(is_match(".*", ""));
    assert!(is_match(".*", "abc"));
    assert!(is_match(".*", "🔥"));
}

#[test]
fn classic_example() {
    assert!(is_match("c*a*b", "aab"));
    assert!(!is_match("mis*is*p*.", "mississippi"));
}
