//go:build !linux

package main

// armParentDeathSignal is a no-op on non-Linux platforms. Stdio MCP clients
// close stdin on exit, which causes the SDK transport's read loop to return
// and Run() to unblock — sufficient for clean shutdown.
func armParentDeathSignal() {}
