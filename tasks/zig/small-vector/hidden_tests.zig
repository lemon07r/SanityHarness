const std = @import("std");
const SmallVec = @import("small_vector.zig").SmallVec;

test "appending past inline capacity allocates" {
    var failing = std.testing.FailingAllocator.init(std.testing.allocator, .{ .fail_index = 0 });
    const alloc = failing.allocator();

    var vec = SmallVec(u8, 2).init(alloc);
    defer vec.deinit();

    try vec.append(1);
    try vec.append(2);

    try std.testing.expectError(error.OutOfMemory, vec.append(3));
}

test "inline capacity zero allocates immediately" {
    var failing = std.testing.FailingAllocator.init(std.testing.allocator, .{ .fail_index = 0 });
    const alloc = failing.allocator();

    var vec = SmallVec(u8, 0).init(alloc);
    defer vec.deinit();

    try std.testing.expectError(error.OutOfMemory, vec.append(1));
}
