use std::collections::HashMap;

use macros::{count_args, hashmap, vec_of};

#[test]
fn hashmap_empty() {
    let map: HashMap<&str, i32> = hashmap!{};
    assert!(map.is_empty());
}

#[test]
fn hashmap_single() {
    let map = hashmap!{
        "one" => 1,
    };
    assert_eq!(map.get("one"), Some(&1));
}

#[test]
fn hashmap_multiple() {
    let map = hashmap!{
        "one" => 1,
        "two" => 2,
        "three" => 3,
    };
    assert_eq!(map.len(), 3);
    assert_eq!(map.get("one"), Some(&1));
    assert_eq!(map.get("two"), Some(&2));
    assert_eq!(map.get("three"), Some(&3));
}

#[test]
fn vec_of_zeros() {
    let v = vec_of![0; 5];
    assert_eq!(v, vec![0, 0, 0, 0, 0]);
}

#[test]
fn vec_of_strings() {
    let v = vec_of![String::from("hello"); 3];
    assert_eq!(v.len(), 3);
    for s in &v {
        assert_eq!(s, "hello");
    }
}

#[test]
fn count_args_zero() {
    assert_eq!(count_args!(), 0);
}

#[test]
fn count_args_one() {
    assert_eq!(count_args!(a), 1);
}

#[test]
fn count_args_many() {
    assert_eq!(count_args!(a, b, c, d, e), 5);
}
