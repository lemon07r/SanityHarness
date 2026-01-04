const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const arena_module = b.addModule("arena", .{
        .root_source_file = b.path("arena.zig"),
    });

    // Main tests
    const main_tests = b.addTest(.{
        .root_source_file = b.path("tests.zig"),
        .target = target,
        .optimize = optimize,
    });
    main_tests.root_module.addImport("arena", arena_module);

    // Hidden tests
    const hidden_tests = b.addTest(.{
        .root_source_file = b.path("hidden_tests.zig"),
        .target = target,
        .optimize = optimize,
    });
    hidden_tests.root_module.addImport("arena", arena_module);

    const run_main_tests = b.addRunArtifact(main_tests);
    const run_hidden_tests = b.addRunArtifact(hidden_tests);

    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_main_tests.step);
    test_step.dependOn(&run_hidden_tests.step);
}
