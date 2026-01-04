const std = @import("std");
const Arena = @import("arena.zig").Arena;

test "init and deinit" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();
}

test "basic allocation" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const bytes = arena.alloc(u8, 100) orelse unreachable;
    try std.testing.expectEqual(@as(usize, 100), bytes.len);
}

test "multiple allocations" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const a = arena.alloc(u8, 100) orelse unreachable;
    const b = arena.alloc(u8, 200) orelse unreachable;
    const c = arena.alloc(u8, 50) orelse unreachable;

    try std.testing.expectEqual(@as(usize, 100), a.len);
    try std.testing.expectEqual(@as(usize, 200), b.len);
    try std.testing.expectEqual(@as(usize, 50), c.len);

    // Allocations should not overlap
    const a_end = @intFromPtr(a.ptr) + a.len;
    const b_start = @intFromPtr(b.ptr);
    try std.testing.expect(a_end <= b_start);
}

test "allocation failure when full" {
    var buffer: [100]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    _ = arena.alloc(u8, 50) orelse unreachable;
    const result = arena.alloc(u8, 100);
    try std.testing.expectEqual(@as(?[]u8, null), result);
}

test "reset allows reuse" {
    var buffer: [100]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    _ = arena.alloc(u8, 80) orelse unreachable;
    arena.reset();
    const after_reset = arena.alloc(u8, 80);
    try std.testing.expect(after_reset != null);
}

test "typed allocation" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const ints = arena.alloc(u32, 10) orelse unreachable;
    try std.testing.expectEqual(@as(usize, 10), ints.len);

    // Write and read back
    for (ints, 0..) |*v, i| {
        v.* = @intCast(i * 2);
    }
    try std.testing.expectEqual(@as(u32, 6), ints[3]);
}

test "allocator interface" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const alloc = arena.allocator();

    var list = std.ArrayList(u32).init(alloc);
    defer list.deinit();

    try list.append(1);
    try list.append(2);
    try list.append(3);

    try std.testing.expectEqual(@as(usize, 3), list.items.len);
}
