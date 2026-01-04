const std = @import("std");
const json = @import("json.zig");

// Hidden tests require additional features

test "parse nested object" {
    const result = json.parse(
        \\{"person": {"name": "Alice", "age": 30}}
    );
    try std.testing.expectEqual(@as(usize, 1), result.object.len);
    const person = result.object[0].value;
    try std.testing.expectEqual(@as(usize, 2), person.object.len);
}

test "parse mixed array" {
    const result = json.parse(
        \\[1, "two", true, null]
    );
    try std.testing.expectEqual(@as(usize, 4), result.array.len);
    try std.testing.expectEqual(@as(i64, 1), result.array[0].integer);
    try std.testing.expectEqualStrings("two", result.array[1].string);
    try std.testing.expectEqual(true, result.array[2].bool);
    try std.testing.expectEqual(json.JsonValue.null, result.array[3]);
}

test "parse string with escapes" {
    const result = json.parse("\"hello\\nworld\"");
    try std.testing.expectEqualStrings("hello\nworld", result.string);
}

test "parse string with unicode" {
    const result = json.parse("\"hello \\u0041\"");
    try std.testing.expectEqualStrings("hello A", result.string);
}

test "parse float" {
    const result = json.parse("3.14");
    try std.testing.expect(result == .float);
    try std.testing.expectApproxEqAbs(@as(f64, 3.14), result.float, 0.001);
}

test "parse scientific notation" {
    const result = json.parse("1.5e10");
    try std.testing.expect(result == .float);
    try std.testing.expectApproxEqAbs(@as(f64, 1.5e10), result.float, 1e6);
}

test "Schema with nested object" {
    const Person = json.Schema(
        \\{"name": "string", "address": {"city": "string", "zip": "int"}}
    );

    const Address = @TypeOf(@as(Person, undefined).address);
    _ = Address;

    // Verify nested struct was created
    try std.testing.expect(@hasField(Person, "name"));
    try std.testing.expect(@hasField(Person, "address"));
}

test "Schema with array field" {
    const Person = json.Schema(
        \\{"name": "string", "scores": ["int"]}
    );

    // scores should be a slice of integers
    const scores_type = @TypeOf(@as(Person, undefined).scores);
    try std.testing.expect(@typeInfo(scores_type) == .Pointer);
}

test "Schema with optional field" {
    const Config = json.Schema(
        \\{"required_field": "string", "optional_field?": "int"}
    );

    // Optional field should be nullable
    const opt_type = @TypeOf(@as(Config, undefined).optional_field);
    try std.testing.expect(@typeInfo(opt_type) == .Optional);
}

test "Schema with default value" {
    const Settings = json.Schema(
        \\{"timeout": {"type": "int", "default": 30}}
    );

    // Should have a default value mechanism
    const s = Settings{};
    try std.testing.expectEqual(@as(i64, 30), s.timeout);
}

test "parse deeply nested" {
    const result = json.parse(
        \\{"a": {"b": {"c": {"d": 42}}}}
    );
    const a = result.object[0].value;
    const b = a.object[0].value;
    const c = b.object[0].value;
    const d = c.object[0].value;
    try std.testing.expectEqual(@as(i64, 42), d.integer);
}

test "stringify array" {
    const arr = json.JsonValue{ .array = &[_]json.JsonValue{
        .{ .integer = 1 },
        .{ .integer = 2 },
        .{ .integer = 3 },
    } };
    const result = json.stringify(arr);
    try std.testing.expectEqualStrings("[1,2,3]", result);
}

test "stringify object" {
    const obj = json.JsonValue{ .object = &[_]struct { key: []const u8, value: json.JsonValue }{
        .{ .key = "a", .value = .{ .integer = 1 } },
    } };
    const result = json.stringify(obj);
    try std.testing.expectEqualStrings("{\"a\":1}", result);
}

test "Schema generates parse function" {
    const Person = json.Schema(
        \\{"name": "string", "age": "int"}
    );

    // Hidden requirement: Schema should generate a parse function
    if (@hasDecl(Person, "parse")) {
        const p = Person.parse(
            \\{"name": "Bob", "age": 25}
        );
        try std.testing.expectEqualStrings("Bob", p.name);
        try std.testing.expectEqual(@as(i64, 25), p.age);
    }
}

test "whitespace handling" {
    const result = json.parse(
        \\{
        \\  "key" : "value" ,
        \\  "num" : 123
        \\}
    );
    try std.testing.expectEqual(@as(usize, 2), result.object.len);
}
