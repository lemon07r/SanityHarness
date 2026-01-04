const std = @import("std");
const SmallVec = @import("small_vector.zig").SmallVec;

test "inline append does not allocate" {
    var failing = std.testing.FailingAllocator.init(std.testing.allocator, .{ .fail_index = 0 });
    const alloc = failing.allocator();

    var vec = SmallVec(u8, 4).init(alloc);
    defer vec.deinit();

    try vec.append(1);
    try vec.append(2);
    try vec.append(3);
    try vec.append(4);

    try std.testing.expectEqual(@as(usize, 4), vec.items().len);
    try std.testing.expectEqualSlices(u8, &[_]u8{ 1, 2, 3, 4 }, vec.items());
}

test "grows on heap and preserves items" {
    var vec = SmallVec(u32, 2).init(std.testing.allocator);
    defer vec.deinit();

    try vec.append(10);
    try vec.append(20);
    try vec.append(30);
    try vec.append(40);

    try std.testing.expectEqual(@as(usize, 4), vec.items().len);
    try std.testing.expectEqualSlices(u32, &[_]u32{ 10, 20, 30, 40 }, vec.items());
}
