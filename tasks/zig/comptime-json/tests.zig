const std = @import("std");
const json = @import("json.zig");

test "parse null" {
    const result = json.parse("null");
    try std.testing.expectEqual(json.JsonValue.null, result);
}

test "parse bool true" {
    const result = json.parse("true");
    try std.testing.expectEqual(json.JsonValue{ .bool = true }, result);
}

test "parse bool false" {
    const result = json.parse("false");
    try std.testing.expectEqual(json.JsonValue{ .bool = false }, result);
}

test "parse integer" {
    const result = json.parse("42");
    try std.testing.expectEqual(json.JsonValue{ .integer = 42 }, result);
}

test "parse negative integer" {
    const result = json.parse("-123");
    try std.testing.expectEqual(json.JsonValue{ .integer = -123 }, result);
}

test "parse string" {
    const result = json.parse("\"hello\"");
    try std.testing.expectEqualStrings("hello", result.string);
}

test "parse empty array" {
    const result = json.parse("[]");
    try std.testing.expectEqual(@as(usize, 0), result.array.len);
}

test "parse array with values" {
    const result = json.parse("[1, 2, 3]");
    try std.testing.expectEqual(@as(usize, 3), result.array.len);
    try std.testing.expectEqual(@as(i64, 1), result.array[0].integer);
    try std.testing.expectEqual(@as(i64, 2), result.array[1].integer);
    try std.testing.expectEqual(@as(i64, 3), result.array[2].integer);
}

test "parse empty object" {
    const result = json.parse("{}");
    try std.testing.expectEqual(@as(usize, 0), result.object.len);
}

test "parse simple object" {
    const result = json.parse(
        \\{"name": "test", "value": 42}
    );
    try std.testing.expectEqual(@as(usize, 2), result.object.len);
}

test "stringify null" {
    const result = json.stringify(.null);
    try std.testing.expectEqualStrings("null", result);
}

test "stringify bool" {
    try std.testing.expectEqualStrings("true", json.stringify(.{ .bool = true }));
    try std.testing.expectEqualStrings("false", json.stringify(.{ .bool = false }));
}

test "stringify integer" {
    const result = json.stringify(.{ .integer = 42 });
    try std.testing.expectEqualStrings("42", result);
}

test "Schema generates struct" {
    const Person = json.Schema(
        \\{"name": "string", "age": "int"}
    );

    const p = Person{ .name = "Alice", .age = 30 };
    try std.testing.expectEqualStrings("Alice", p.name);
    try std.testing.expectEqual(@as(i64, 30), p.age);
}
