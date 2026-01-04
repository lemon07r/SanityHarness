const std = @import("std");

fn fileExists(path: []const u8) bool {
    std.fs.cwd().access(path, .{}) catch |err| switch (err) {
        error.FileNotFound => return false,
        else => @panic("failed to check file existence"),
    };
    return true;
}

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const json_module = b.addModule("json", .{
        .root_source_file = b.path("json.zig"),
    });

    const main_tests = b.addTest(.{
        .root_source_file = b.path("tests.zig"),
        .target = target,
        .optimize = optimize,
    });
    main_tests.root_module.addImport("json", json_module);

    const run_main_tests = b.addRunArtifact(main_tests);

    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_main_tests.step);

    if (fileExists("hidden_tests.zig")) {
        const hidden_tests = b.addTest(.{
            .root_source_file = b.path("hidden_tests.zig"),
            .target = target,
            .optimize = optimize,
        });
        hidden_tests.root_module.addImport("json", json_module);

        const run_hidden_tests = b.addRunArtifact(hidden_tests);
        test_step.dependOn(&run_hidden_tests.step);
    }
}
