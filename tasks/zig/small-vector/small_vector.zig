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
            return Self{
                .alloc = alloc,
                .inline_buf = undefined,
                .len = 0,
                .heap = null,
            };
        }

        pub fn deinit(self: *Self) void {
            if (self.heap) |heap_slice| {
                self.alloc.free(heap_slice);
            }
        }

        pub fn append(self: *Self, value: T) !void {
            if (self.len < InlineCap) {
                // Still in inline storage
                self.inline_buf[self.len] = value;
                self.len += 1;
            } else if (self.heap) |*heap_slice| {
                // Already on heap, realloc to grow
                const new_slice = try self.alloc.realloc(heap_slice.*, self.len + 1);
                new_slice[self.len] = value;
                heap_slice.* = new_slice;
                self.len += 1;
            } else {
                // First spill to heap
                const new_heap = try self.alloc.alloc(T, self.len + 1);
                // Copy inline items to heap
                @memcpy(new_heap[0..self.len], self.inline_buf[0..self.len]);
                new_heap[self.len] = value;
                self.heap = new_heap;
                self.len += 1;
            }
        }

        pub fn items(self: *Self) []T {
            if (self.heap) |heap_slice| {
                return heap_slice[0..self.len];
            }
            return self.inline_buf[0..self.len];
        }
    };
}
