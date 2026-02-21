const std = @import("std");

/// Comptime JSON value type
pub const JsonValue = union(enum) {
    null,
    bool: bool,
    integer: i64,
    float: f64,
    string: []const u8,
    array: []const JsonValue,
    object: []const struct { key: []const u8, value: JsonValue },
};

/// Parses a JSON string at compile time and returns the parsed value.
/// The input must be a comptime-known string literal.
///
/// Required parsing behavior:
/// - Support `null`, booleans, integers, floats, and scientific notation.
/// - Support strings with escapes (including `\n`, `\t`, `\"`, `\\`, `\uXXXX`).
/// - Support arrays and objects, including nested combinations.
/// - Ignore insignificant JSON whitespace around tokens.
pub fn parse(comptime json: []const u8) JsonValue {
    _ = json;
    @compileError("Please implement parse");
}

/// Stringifies a `JsonValue` back to JSON format at compile time.
/// Output must be valid JSON for all `JsonValue` variants.
pub fn stringify(comptime value: JsonValue) []const u8 {
    _ = value;
    @compileError("Please implement stringify");
}

/// Creates a struct type from a JSON object schema at compile time.
/// The schema should be a JSON object where keys are field names
/// and values describe the field types.
///
/// Example schema: {"name": "string", "age": "int", "active": "bool"}
/// Generates: struct { name: []const u8, age: i64, active: bool }
///
/// Required schema features:
/// - Nested object descriptors generate nested struct fields.
/// - Single-element array descriptors (for example `["int"]`) generate slice
///   fields of the described element type.
/// - Field names ending with `?` are optional fields in the generated struct.
/// - Descriptor objects like `{"type":"int","default":30}` create fields with
///   default values.
/// - Generated struct types must provide a `parse` declaration that performs
///   type-safe parsing from JSON into that struct type.
pub fn Schema(comptime json_schema: []const u8) type {
    _ = json_schema;
    @compileError("Please implement Schema");
}
