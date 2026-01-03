use generational_arena::Arena;

#[test]
fn stale_handles_do_not_resurrect_values() {
    let mut arena = Arena::new();

    let h1 = arena.insert(10);
    let idx = h1.index();

    assert_eq!(arena.remove(h1), Some(10));

    // Reuse the same slot multiple times.
    let h2 = arena.insert(20);
    assert_eq!(h2.index(), idx);
    assert_eq!(arena.remove(h2), Some(20));

    let h3 = arena.insert(30);
    assert_eq!(h3.index(), idx);

    assert_eq!(arena.get(h1), None);
    assert_eq!(arena.get(h2), None);
    assert_eq!(arena.get(h3), Some(&30));
}

#[test]
fn len_tracks_live_values_only() {
    let mut arena = Arena::new();
    let a = arena.insert(1);
    let b = arena.insert(2);
    let c = arena.insert(3);
    assert_eq!(arena.len(), 3);

    arena.remove(b);
    assert_eq!(arena.len(), 2);

    arena.remove(a);
    arena.remove(c);
    assert_eq!(arena.len(), 0);
    assert!(arena.is_empty());
}
