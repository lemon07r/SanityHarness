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
