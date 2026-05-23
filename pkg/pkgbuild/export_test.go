// Package pkgbuild — test-only exports for unexported functions.
package pkgbuild

// ApkInstalledSetForTesting exposes apkInstalledSet for unit tests.
func ApkInstalledSetForTesting(path string) map[string]bool {
	return apkInstalledSet(path)
}

// PacmanDirToNameForTesting exposes pacmanDirToName for unit tests.
func PacmanDirToNameForTesting(dir string) string {
	return pacmanDirToName(dir)
}

// StripVersionConstraintForTesting exposes stripVersionConstraint for unit tests.
func StripVersionConstraintForTesting(spec string) string {
	return stripVersionConstraint(spec)
}
