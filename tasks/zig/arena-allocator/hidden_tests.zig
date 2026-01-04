const std = @import("std");
const Arena = @import("arena.zig").Arena;

test "child arena allocation" {
    var buffer: [1024]u8 = undefined;
    var parent = Arena.init(&buffer);
    defer parent.deinit();

    // Allocate some in parent
    _ = parent.alloc(u8, 100) orelse unreachable;

    var child = parent.createChild() orelse unreachable;
    defer child.deinit();

    // Child should be able to allocate from remaining space
    const child_alloc = child.alloc(u8, 200) orelse unreachable;
    try std.testing.expectEqual(@as(usize, 200), child_alloc.len);
}

test "child arena isolation" {
    var buffer: [1024]u8 = undefined;
    var parent = Arena.init(&buffer);
    defer parent.deinit();

    var child = parent.createChild() orelse unreachable;

    // Child allocations
    _ = child.alloc(u8, 100) orelse unreachable;
    _ = child.alloc(u8, 100) orelse unreachable;

    // Deinitializing child should not affect parent
    child.deinit();

    // Parent should still be usable
    const after = parent.alloc(u8, 50);
    try std.testing.expect(after != null);
}

test "aligned allocation" {
    var buffer: [4096]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    // Allocate with 64-byte alignment
    const aligned = arena.allocAligned(u8, 100, 64) orelse unreachable;
    try std.testing.expectEqual(@as(usize, 0), @intFromPtr(aligned.ptr) % 64);

    // Allocate with 256-byte alignment
    const aligned256 = arena.allocAligned(u8, 50, 256) orelse unreachable;
    try std.testing.expectEqual(@as(usize, 0), @intFromPtr(aligned256.ptr) % 256);
}

test "checkpoint save and restore" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    // Allocate some memory
    _ = arena.alloc(u8, 100) orelse unreachable;

    // Save checkpoint
    const checkpoint = arena.saveState();

    // Allocate more
    _ = arena.alloc(u8, 200) orelse unreachable;
    _ = arena.alloc(u8, 150) orelse unreachable;

    // Restore to checkpoint
    arena.restoreState(checkpoint);

    // Should be able to allocate 350 bytes again (200 + 150)
    const after = arena.alloc(u8, 350);
    try std.testing.expect(after != null);
}

test "multiple checkpoints" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const cp1 = arena.saveState();
    _ = arena.alloc(u8, 100) orelse unreachable;

    const cp2 = arena.saveState();
    _ = arena.alloc(u8, 200) orelse unreachable;

    const cp3 = arena.saveState();
    _ = cp3;
    _ = arena.alloc(u8, 300) orelse unreachable;

    // Restore to cp2
    arena.restoreState(cp2);
    const after_cp2 = arena.alloc(u8, 500);
    try std.testing.expect(after_cp2 != null);

    // Restore to cp1
    arena.restoreState(cp1);
    const after_cp1 = arena.alloc(u8, 900);
    try std.testing.expect(after_cp1 != null);
}

test "resize in place" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    // Allocate
    const original = arena.alloc(u8, 100) orelse unreachable;

    // Resize larger (should work if it's the last allocation)
    const resized = arena.resize(u8, original, 200);
    try std.testing.expect(resized != null);
    try std.testing.expectEqual(@as(usize, 200), resized.?.len);

    // Pointers should be the same (in-place resize)
    try std.testing.expectEqual(@intFromPtr(original.ptr), @intFromPtr(resized.?.ptr));
}

test "resize failure when not last" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const first = arena.alloc(u8, 100) orelse unreachable;
    _ = arena.alloc(u8, 100) orelse unreachable; // Another allocation

    // Resizing first should fail (not the last allocation)
    const result = arena.resize(u8, first, 200);
    try std.testing.expectEqual(@as(?[]u8, null), result);
}

test "bytes allocated tracking" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    try std.testing.expectEqual(@as(usize, 0), arena.bytesAllocated());

    _ = arena.alloc(u8, 100) orelse unreachable;
    try std.testing.expect(arena.bytesAllocated() >= 100);

    _ = arena.alloc(u8, 200) orelse unreachable;
    try std.testing.expect(arena.bytesAllocated() >= 300);
}

test "bytes remaining tracking" {
    var buffer: [1024]u8 = undefined;
    var arena = Arena.init(&buffer);
    defer arena.deinit();

    const initial = arena.bytesRemaining();
    try std.testing.expect(initial <= 1024);

    _ = arena.alloc(u8, 100) orelse unreachable;
    try std.testing.expect(arena.bytesRemaining() < initial);
}

test "nested child arenas" {
    var buffer: [4096]u8 = undefined;
    var root = Arena.init(&buffer);
    defer root.deinit();

    var child1 = root.createChild() orelse unreachable;
    _ = child1.alloc(u8, 500) orelse unreachable;

    var grandchild = child1.createChild() orelse unreachable;
    _ = grandchild.alloc(u8, 200) orelse unreachable;

    grandchild.deinit();
    child1.deinit();

    // Root should still work
    const after = root.alloc(u8, 100);
    try std.testing.expect(after != null);
}
