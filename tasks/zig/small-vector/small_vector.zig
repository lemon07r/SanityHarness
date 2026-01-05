const std = @import("std");

/// SmallVec stores up to InlineCap items inline, and spills to the heap when exceeded.
/// Note: InlineCap may be 0, meaning all storage is heap-allocated from the start.
pub fn SmallVec(comptime T: type, comptime InlineCap: usize) type {
    return struct {
        const Self = @This();

        alloc: std.mem.Allocator,
	        inline_buf: [InlineCap]T,
        len: usize,
        heap: ?[]T,

        pub fn init(alloc: std.mem.Allocator) Self {
            _ = alloc;
            @panic("Please implement SmallVec.init");
        }

        pub fn deinit(self: *Self) void {
            _ = self;
            @panic("Please implement SmallVec.deinit");
        }

        pub fn append(self: *Self, value: T) !void {
            _ = self;
            _ = value;
            @panic("Please implement SmallVec.append");
        }

        pub fn items(self: *Self) []T {
            _ = self;
            @panic("Please implement SmallVec.items");
        }
    };
}
