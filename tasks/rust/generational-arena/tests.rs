use generational_arena::Arena;

#[test]
fn new_is_empty() {
    let arena: Arena<i32> = Arena::new();
    assert!(arena.is_empty());
    assert_eq!(arena.len(), 0);
}

#[test]
fn insert_and_get() {
    let mut arena = Arena::new();
    let h = arena.insert("hello".to_string());
    assert_eq!(arena.len(), 1);
    assert_eq!(arena.get(h).map(|s| s.as_str()), Some("hello"));
}

#[test]
fn get_mut_allows_update() {
    let mut arena = Arena::new();
    let h = arena.insert(1);
    *arena.get_mut(h).unwrap() = 2;
    assert_eq!(arena.get(h), Some(&2));
}

#[test]
fn remove_makes_handle_invalid() {
    let mut arena = Arena::new();
    let h = arena.insert(123);
    assert_eq!(arena.remove(h), Some(123));
    assert_eq!(arena.get(h), None);
    assert!(arena.is_empty());
}

#[test]
fn slot_reuse_increments_generation() {
    let mut arena = Arena::new();
    let h1 = arena.insert('a');
    let idx = h1.index();
    let gen = h1.generation();

    assert_eq!(arena.remove(h1), Some('a'));

    let h2 = arena.insert('b');
    assert_eq!(h2.index(), idx);
    assert_eq!(h2.generation(), gen + 1);

    assert_eq!(arena.get(h1), None);
    assert_eq!(arena.get(h2), Some(&'b'));
}
