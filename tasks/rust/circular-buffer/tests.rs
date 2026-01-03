use circular_buffer::*;

#[test]
fn reading_empty_buffer_should_fail() {
    let mut buffer: CircularBuffer<i32> = CircularBuffer::new(1);
    assert_eq!(buffer.read(), Err(Error::EmptyBuffer));
}

#[test]
fn can_read_item_just_written() {
    let mut buffer = CircularBuffer::new(1);
    assert!(buffer.write(1).is_ok());
    assert_eq!(buffer.read(), Ok(1));
}

#[test]
fn each_item_may_only_be_read_once() {
    let mut buffer = CircularBuffer::new(1);
    assert!(buffer.write(1).is_ok());
    assert_eq!(buffer.read(), Ok(1));
    assert_eq!(buffer.read(), Err(Error::EmptyBuffer));
}

#[test]
fn items_are_read_in_order_written() {
    let mut buffer = CircularBuffer::new(2);
    assert!(buffer.write(1).is_ok());
    assert!(buffer.write(2).is_ok());
    assert_eq!(buffer.read(), Ok(1));
    assert_eq!(buffer.read(), Ok(2));
}

#[test]
fn full_buffer_cant_be_written_to() {
    let mut buffer = CircularBuffer::new(1);
    assert!(buffer.write(1).is_ok());
    assert_eq!(buffer.write(2), Err(Error::FullBuffer));
}

#[test]
fn read_frees_up_capacity() {
    let mut buffer = CircularBuffer::new(1);
    assert!(buffer.write(1).is_ok());
    assert_eq!(buffer.read(), Ok(1));
    assert!(buffer.write(2).is_ok());
    assert_eq!(buffer.read(), Ok(2));
}

#[test]
fn read_position_maintained_across_writes() {
    let mut buffer = CircularBuffer::new(3);
    assert!(buffer.write(1).is_ok());
    assert!(buffer.write(2).is_ok());
    assert_eq!(buffer.read(), Ok(1));
    assert!(buffer.write(3).is_ok());
    assert_eq!(buffer.read(), Ok(2));
    assert_eq!(buffer.read(), Ok(3));
}

#[test]
fn clear_frees_buffer() {
    let mut buffer = CircularBuffer::new(1);
    assert!(buffer.write(1).is_ok());
    buffer.clear();
    assert!(buffer.write(2).is_ok());
    assert_eq!(buffer.read(), Ok(2));
}

#[test]
fn overwrite_replaces_oldest() {
    let mut buffer = CircularBuffer::new(2);
    assert!(buffer.write(1).is_ok());
    assert!(buffer.write(2).is_ok());
    buffer.overwrite(3);
    assert_eq!(buffer.read(), Ok(2));
    assert_eq!(buffer.read(), Ok(3));
}

#[test]
fn overwrite_on_empty_buffer() {
    let mut buffer = CircularBuffer::new(1);
    buffer.overwrite(1);
    assert_eq!(buffer.read(), Ok(1));
}
