//go:build !linux

package main

// initRootless is a no-op on non-Linux platforms.
func initRootless() {}
