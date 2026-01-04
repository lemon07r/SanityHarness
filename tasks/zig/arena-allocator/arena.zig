const std = @import("std");

/// A simple arena allocator that allocates memory from a fixed buffer.
/// Memory is freed all at once when the arena is deinitialized.
pub const Arena = struct {
    buffer: []u8,
    offset: usize,

    /// Creates a new arena backed by the given buffer.
    pub fn init(buffer: []u8) Arena {
        _ = buffer;
        @panic("Please implement Arena.init");
    }

    /// Deinitializes the arena. After this, the arena should not be used.
    pub fn deinit(self: *Arena) void {
        _ = self;
        @panic("Please implement Arena.deinit");
    }

    /// Allocates `n` bytes from the arena.
    /// Returns null if there is not enough space.
    pub fn alloc(self: *Arena, comptime T: type, n: usize) ?[]T {
        _ = self;
        _ = n;
        @panic("Please implement Arena.alloc");
    }

    /// Resets the arena, allowing all previously allocated memory to be reused.
    pub fn reset(self: *Arena) void {
        _ = self;
        @panic("Please implement Arena.reset");
    }

    /// Returns the allocator interface for this arena.
    pub fn allocator(self: *Arena) std.mem.Allocator {
        _ = self;
        @panic("Please implement Arena.allocator");
    }
};
