use macros::{count_args, hashmap, vec_of};
use std::collections::HashMap;

#[test]
fn hashmap_accepts_trailing_comma_and_expressions() {
    let map = hashmap! {
        "one".to_string() => 1 + 0,
        "two".to_string() => 1 + 1,
    };

    assert_eq!(map.get("one"), Some(&1));
    assert_eq!(map.get("two"), Some(&2));
}

#[test]
fn hashmap_can_be_typed_empty() {
    let map: HashMap<i32, i32> = hashmap! {};
    assert!(map.is_empty());
}

#[test]
fn vec_of_clones_distinct_elements() {
    let mut v = vec_of![vec![1]; 3];
    v[0].push(2);
    assert_eq!(v[0], vec![1, 2]);
    assert_eq!(v[1], vec![1]);
    assert_eq!(v[2], vec![1]);
}

#[test]
fn count_args_handles_complex_tokens() {
    assert_eq!(count_args!(a, b + c, (d, e), { let x = 1; x }, "str",), 5);
}
