//go:build linux

package main

import (
	"github.com/M0Rf30/yap/v2/pkg/container/rootless"
)

// initRootless must be called at the very start of main(), before cobra runs.
// If the current process is a rootlesskit child re-execution, it completes
// the child setup and exits — the normal cobra flow never runs in that case.
func initRootless() {
	rootless.MaybeRunAsChild()
}
