// Package tasks provides the embedded task files.
package tasks

import "embed"

// FS contains all embedded task files.
//
//go:embed all:go all:rust all:typescript
var FS embed.FS
