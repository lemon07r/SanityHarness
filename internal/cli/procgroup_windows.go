//go:build windows

package cli

import "os/exec"

// setupProcessGroup is a no-op on Windows. Process group management is not
// supported in the same way; the context cancellation will still kill the
// direct child process.
func setupProcessGroup(_ *exec.Cmd) {}
