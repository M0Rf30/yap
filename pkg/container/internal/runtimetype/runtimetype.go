// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package runtimetype defines the RuntimeType enum shared between the
// container package and its sub-packages to avoid import cycles.
package runtimetype

// RuntimeType identifies which container backend is in use.
type RuntimeType string

const (
	// CLI uses the system podman or docker binary.
	CLI RuntimeType = "cli"
	// Rootless uses the built-in rootless runner (go-containerregistry + rootlesskit).
	Rootless RuntimeType = "rootless"
)
