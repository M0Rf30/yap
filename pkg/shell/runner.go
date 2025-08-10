// Package shell provides process execution and command running utilities.
package shell

import (
	"context"
	"os/exec"
)

// Runner provides methods for executing shell commands.
type Runner struct {
	// Add fields for configuration if needed in the future
}

// NewRunner creates a new shell command runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Exec executes a command in the specified directory.
func (r *Runner) Exec(excludeStdout bool, dir, name string, args ...string) error {
	return r.ExecContext(context.Background(), excludeStdout, dir, name, args...)
}

// ExecContext executes a command with context support.
func (r *Runner) ExecContext(ctx context.Context, excludeStdout bool, dir, name string,
	args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	// TODO: Integrate output handling with logging system when it's standardized
	_ = excludeStdout // Placeholder to avoid unused parameter

	if dir != "" {
		cmd.Dir = dir
	}

	return cmd.Run()
}

// Global runner instance for backward compatibility
var globalRunner = NewRunner()

// Exec is a convenience function that uses the global runner.
func Exec(excludeStdout bool, dir, name string, args ...string) error {
	return globalRunner.Exec(excludeStdout, dir, name, args...)
}

// ExecContext is a convenience function that uses the global runner with context.
func ExecContext(ctx context.Context, excludeStdout bool, dir, name string,
	args ...string) error {
	return globalRunner.ExecContext(ctx, excludeStdout, dir, name, args...)
}
