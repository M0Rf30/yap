// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//go:build !linux

package container

import (
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// NewRootlessRuntime is not supported on non-Linux platforms.
func NewRootlessRuntime() (Runtime, error) {
	return nil, errors.New(errors.ErrTypeConfiguration,
		"rootless runtime is only supported on Linux").
		WithOperation("NewRootlessRuntime")
}
