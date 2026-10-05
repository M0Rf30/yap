// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//go:build !linux

package main

// initRootless is a no-op on non-Linux platforms.
func initRootless() {}
