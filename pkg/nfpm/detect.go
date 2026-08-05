package nfpm

import (
	"os"
	"path/filepath"
	"slices"
)

// SpecFileNames is the ordered list of recognised nfpm spec filenames,
// checked in order by FindSpec.
var SpecFileNames = []string{"nfpm.yaml", "nfpm.yml", ".nfpm.yaml", ".nfpm.yml"}

// FindSpec returns the first recognised nfpm spec file in dir and true, or
// ("", false) when none of SpecFileNames exists in dir.
func FindSpec(dir string) (string, bool) {
	for _, name := range SpecFileNames {
		candidate := filepath.Join(dir, name)

		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}

	return "", false
}

// IsSpecFile reports whether the base name of path is a recognised nfpm
// spec filename.
func IsSpecFile(path string) bool {
	return slices.Contains(SpecFileNames, filepath.Base(path))
}
