use std::ptr::NonNull;

/// A doubly-linked list node.
struct Node<T> {
    value: T,
    prev: Option<NonNull<Node<T>>>,
    next: Option<NonNull<Node<T>>>,
}

/// A doubly-linked list.
pub struct DoublyLinkedList<T> {
    head: Option<NonNull<Node<T>>>,
    tail: Option<NonNull<Node<T>>>,
    len: usize,
}

impl<T> DoublyLinkedList<T> {
    /// Creates a new empty list.
    pub fn new() -> Self {
        todo!("Implement new")
    }

    /// Returns the length of the list.
    pub fn len(&self) -> usize {
        todo!("Implement len")
    }

    /// Returns true if the list is empty.
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// Pushes a value to the front of the list.
    pub fn push_front(&mut self, value: T) {
        todo!("Implement push_front")
    }

    /// Pushes a value to the back of the list.
    pub fn push_back(&mut self, value: T) {
        todo!("Implement push_back")
    }

    /// Pops a value from the front of the list.
    pub fn pop_front(&mut self) -> Option<T> {
        todo!("Implement pop_front")
    }

    /// Pops a value from the back of the list.
    pub fn pop_back(&mut self) -> Option<T> {
        todo!("Implement pop_back")
    }

    /// Returns a reference to the front value.
    pub fn front(&self) -> Option<&T> {
        todo!("Implement front")
    }

    /// Returns a reference to the back value.
    pub fn back(&self) -> Option<&T> {
        todo!("Implement back")
    }
}

impl<T> Default for DoublyLinkedList<T> {
    fn default() -> Self {
        Self::new()
    }
}

impl<T> Drop for DoublyLinkedList<T> {
    fn drop(&mut self) {
        // TODO: Properly deallocate all nodes to avoid memory leaks
        while self.pop_front().is_some() {}
    }
}
