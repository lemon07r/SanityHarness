/// A circular buffer with fixed capacity.
pub struct CircularBuffer<T> {
    // TODO: Add fields
    _marker: std::marker::PhantomData<T>,
}

#[derive(Debug, PartialEq, Eq)]
pub enum Error {
    EmptyBuffer,
    FullBuffer,
}

impl<T> CircularBuffer<T> {
    /// Creates a new empty circular buffer with the given capacity.
    pub fn new(capacity: usize) -> Self {
        todo!("Implement new with capacity {}", capacity)
    }

    /// Writes an element to the buffer.
    /// Returns an error if the buffer is full.
    pub fn write(&mut self, element: T) -> Result<(), Error> {
        todo!("Implement write")
    }

    /// Reads the oldest element from the buffer.
    /// Returns an error if the buffer is empty.
    pub fn read(&mut self) -> Result<T, Error> {
        todo!("Implement read")
    }

    /// Clears the buffer.
    pub fn clear(&mut self) {
        todo!("Implement clear")
    }

    /// Writes an element, overwriting the oldest if full.
    pub fn overwrite(&mut self, element: T) {
        todo!("Implement overwrite")
    }

    /// Returns true if the buffer is empty.
    pub fn is_empty(&self) -> bool {
        todo!("Implement is_empty")
    }

    /// Returns true if the buffer is full.
    pub fn is_full(&self) -> bool {
        todo!("Implement is_full")
    }
}
