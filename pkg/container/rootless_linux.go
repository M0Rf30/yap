//go:build linux

package container

import (
	"github.com/M0Rf30/yap/v2/pkg/container/rootless"
)

// NewRootlessRuntime returns the built-in rootless Runtime (Linux only).
func NewRootlessRuntime() (Runtime, error) {
	return rootless.NewRuntime(), nil
}
