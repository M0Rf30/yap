// Package testhelpers provides shared test fixtures for package builder tests.
package testhelpers

import "github.com/M0Rf30/yap/v2/pkg/pkgbuild"

// BasePKGBUILD returns a minimal *pkgbuild.PKGBUILD suitable for unit tests.
// It covers the fields common to all four builder formats (APK, DEB, RPM, Pacman).
// Use the returned pointer directly or apply format-specific overrides before use.
func BasePKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		Arch:         []string{"x86_64"},
		ArchComputed: "x86_64",
		PkgDesc:      "Test package description",
		Maintainer:   "test@example.com",
		License:      []string{"MIT"},
		StripEnabled: false,
	}
}
