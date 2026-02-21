/// Creates a HashMap from key-value pairs.
/// 
/// Example:
/// ```
/// # use macros::hashmap;
/// let map = hashmap!{
///     "a" => 1,
///     "b" => 2,
/// };
/// ```
#[macro_export]
macro_rules! hashmap {
    // TODO: Implement this macro
    ($($key:expr => $value:expr),* $(,)?) => {
        compile_error!("Please implement the hashmap! macro")
    };
}

/// Creates a Vec with repeated elements.
///
/// Example:
/// ```
/// # use macros::vec_of;
/// let v = vec_of![0; 5]; // [0, 0, 0, 0, 0]
/// ```
#[macro_export]
macro_rules! vec_of {
    // TODO: Implement this macro
    ($elem:expr; $n:expr) => {
        compile_error!("Please implement the vec_of! macro")
    };
}

/// A macro that counts its arguments.
///
/// Counts top-level, comma-separated Rust expressions.
/// Commas that are part of an expression (for example in tuples, blocks, or
/// function calls) do not split arguments.
///
/// Example:
/// ```
/// # use macros::count_args;
/// assert_eq!(count_args!(a, b, c), 3);
/// ```
#[macro_export]
macro_rules! count_args {
    // TODO: Implement this macro
    ($($args:tt)*) => {
        compile_error!("Please implement the count_args! macro")
    };
}
