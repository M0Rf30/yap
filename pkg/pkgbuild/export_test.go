// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pkgbuild — test-only exports for unexported functions.
package pkgbuild

// ApkInstalledSetForTesting exposes apkInstalledSet for unit tests.
func ApkInstalledSetForTesting(root string) (map[string]bool, error) {
	return apkInstalledSet(root)
}

// PacmanInstalledSetForTesting exposes pacmanInstalledSet for unit tests.
func PacmanInstalledSetForTesting(root string) (map[string]bool, error) {
	return pacmanInstalledSet(root)
}

// StripVersionConstraintForTesting exposes stripVersionConstraint for unit tests.
func StripVersionConstraintForTesting(spec string) string {
	return stripVersionConstraint(spec)
}
