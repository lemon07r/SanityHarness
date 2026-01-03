use doubly_linked_list::DoublyLinkedList;

#[test]
fn new_list_is_empty() {
    let list: DoublyLinkedList<i32> = DoublyLinkedList::new();
    assert!(list.is_empty());
    assert_eq!(list.len(), 0);
}

#[test]
fn push_front_increases_len() {
    let mut list = DoublyLinkedList::new();
    list.push_front(1);
    assert_eq!(list.len(), 1);
    list.push_front(2);
    assert_eq!(list.len(), 2);
}

#[test]
fn push_back_increases_len() {
    let mut list = DoublyLinkedList::new();
    list.push_back(1);
    assert_eq!(list.len(), 1);
    list.push_back(2);
    assert_eq!(list.len(), 2);
}

#[test]
fn pop_front_returns_front() {
    let mut list = DoublyLinkedList::new();
    list.push_front(1);
    list.push_front(2);
    assert_eq!(list.pop_front(), Some(2));
    assert_eq!(list.pop_front(), Some(1));
    assert_eq!(list.pop_front(), None);
}

#[test]
fn pop_back_returns_back() {
    let mut list = DoublyLinkedList::new();
    list.push_back(1);
    list.push_back(2);
    assert_eq!(list.pop_back(), Some(2));
    assert_eq!(list.pop_back(), Some(1));
    assert_eq!(list.pop_back(), None);
}

#[test]
fn front_returns_front_value() {
    let mut list = DoublyLinkedList::new();
    assert_eq!(list.front(), None);
    list.push_front(1);
    assert_eq!(list.front(), Some(&1));
    list.push_front(2);
    assert_eq!(list.front(), Some(&2));
}

#[test]
fn back_returns_back_value() {
    let mut list = DoublyLinkedList::new();
    assert_eq!(list.back(), None);
    list.push_back(1);
    assert_eq!(list.back(), Some(&1));
    list.push_back(2);
    assert_eq!(list.back(), Some(&2));
}

#[test]
fn mixed_operations() {
    let mut list = DoublyLinkedList::new();
    list.push_front(1);
    list.push_back(2);
    list.push_front(0);
    // List: 0 <-> 1 <-> 2
    assert_eq!(list.pop_front(), Some(0));
    assert_eq!(list.pop_back(), Some(2));
    assert_eq!(list.pop_front(), Some(1));
    assert!(list.is_empty());
}

#[test]
fn single_element_operations() {
    let mut list = DoublyLinkedList::new();
    list.push_front(1);
    assert_eq!(list.front(), Some(&1));
    assert_eq!(list.back(), Some(&1));
    assert_eq!(list.pop_back(), Some(1));
    assert!(list.is_empty());
}
