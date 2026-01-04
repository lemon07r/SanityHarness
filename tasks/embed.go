// Package tasks provides the embedded task files.
package tasks

import "embed"

// FS contains all embedded task files.
//
//go:embed all:go all:rust all:typescript all:kotlin all:dart all:zig
var FS embed.FS
