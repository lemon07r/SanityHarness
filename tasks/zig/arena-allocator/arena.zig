const std = @import("std");

/// A simple arena allocator that allocates memory from a fixed buffer.
/// Memory is freed all at once when the arena is deinitialized.
///
/// Behavioral requirements:
/// - Allocation is bump-pointer style inside the provided `buffer`.
/// - `alloc`/`allocAligned` return null when space is insufficient.
/// - `allocAligned` must satisfy the requested alignment.
/// - Child arenas allocate from parent remaining space without invalidating the
///   parent on child deinit.
/// - `saveState`/`restoreState` must support rolling back allocations.
/// - `resize` may only succeed for resizable allocations under arena rules.
/// - `allocator()` must expose this arena via `std.mem.Allocator`.
pub const Arena = struct {
    buffer: []u8,
    offset: usize,

    /// A checkpoint representing a saved state of the arena.
    pub const Checkpoint = struct {
        offset: usize,
    };

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

    /// Allocates memory with a specific alignment.
    /// Returns null if there is not enough space.
    pub fn allocAligned(self: *Arena, comptime T: type, n: usize, alignment: u29) ?[]T {
        _ = self;
        _ = n;
        _ = alignment;
        @panic("Please implement Arena.allocAligned");
    }

    /// Creates a child arena that allocates from the parent's remaining space.
    /// The child must be deinitialized before the parent.
    pub fn createChild(self: *Arena) ?Arena {
        _ = self;
        @panic("Please implement Arena.createChild");
    }

    /// Saves the current state of the arena.
    pub fn saveState(self: *Arena) Checkpoint {
        _ = self;
        @panic("Please implement Arena.saveState");
    }

    /// Restores the arena to a previously saved state.
    /// All allocations made after the checkpoint are invalidated.
    pub fn restoreState(self: *Arena, checkpoint: Checkpoint) void {
        _ = self;
        _ = checkpoint;
        @panic("Please implement Arena.restoreState");
    }

    /// Attempts to resize an existing allocation.
    /// Returns the new slice if successful, null otherwise.
    pub fn resize(self: *Arena, comptime T: type, old: []T, new_len: usize) ?[]T {
        _ = self;
        _ = old;
        _ = new_len;
        @panic("Please implement Arena.resize");
    }

    /// Returns the number of bytes currently allocated.
    pub fn bytesAllocated(self: *Arena) usize {
        _ = self;
        @panic("Please implement Arena.bytesAllocated");
    }

    /// Returns the number of bytes remaining.
    pub fn bytesRemaining(self: *Arena) usize {
        _ = self;
        @panic("Please implement Arena.bytesRemaining");
    }

    /// Resets the arena, allowing all previously allocated memory to be reused.
    pub fn reset(self: *Arena) void {
        _ = self;
        @panic("Please implement Arena.reset");
    }

    /// Returns the allocator interface for this arena.
    /// The adapter must route allocator operations to this arena's state.
    pub fn allocator(self: *Arena) std.mem.Allocator {
        _ = self;
        @panic("Please implement Arena.allocator");
    }
};
