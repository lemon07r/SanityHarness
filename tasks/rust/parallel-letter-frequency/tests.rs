use std::collections::HashMap;
use parallel_letter_frequency::frequency;

#[test]
fn no_texts() {
    assert_eq!(frequency(&[], 4), HashMap::new());
}

#[test]
fn one_letter() {
    let mut expected = HashMap::new();
    expected.insert('a', 1);
    assert_eq!(frequency(&["a"], 4), expected);
}

#[test]
fn case_insensitive() {
    let mut expected = HashMap::new();
    expected.insert('a', 2);
    assert_eq!(frequency(&["aA"], 4), expected);
}

#[test]
fn many_texts() {
    let texts = ["aaa", "bbb", "ccc"];
    let result = frequency(&texts, 4);
    assert_eq!(result.get(&'a'), Some(&3));
    assert_eq!(result.get(&'b'), Some(&3));
    assert_eq!(result.get(&'c'), Some(&3));
}

#[test]
fn ignores_non_letters() {
    let texts = ["1234", "!@#$", "   "];
    let result = frequency(&texts, 4);
    assert!(result.is_empty());
}

#[test]
fn unicode_letters() {
    let texts = ["aeiou\u{00FC}"];
    let result = frequency(&texts, 1);
    assert_eq!(result.get(&'\u{00FC}'), Some(&1));
}

#[test]
fn single_worker() {
    let texts = ["abc", "def", "ghi"];
    let result = frequency(&texts, 1);
    assert_eq!(result.len(), 9);
}

#[test]
fn many_workers() {
    let texts = ["abc", "def", "ghi"];
    let result = frequency(&texts, 10);
    assert_eq!(result.len(), 9);
}
