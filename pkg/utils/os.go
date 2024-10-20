package utils

import (
	"runtime"
)

// GetArchitecture returns the corresponding pacman architecture for the current GOARCH.
func GetArchitecture() string {
	// Create a map of GOARCH values to pacman arch ones
	architectureMap := map[string]string{
		"amd64":   "x86_64",
		"386":     "i686",
		"arm":     "armv7h",  // Common for ARM 32-bit
		"arm64":   "aarch64", // Common for ARM 64-bit
		"ppc64":   "ppc64",
		"ppc64le": "ppc64le",
		"s390x":   "s390x",
		"mips":    "mips",
		"mipsle":  "mipsle",
		"riscv64": "riscv64",
		// Add more mappings as needed
	}

	// Get the current architecture using runtime.GOARCH
	currentArch := runtime.GOARCH

	// Get the corresponding pacman architecture from the map
	pacmanArch := architectureMap[currentArch]

	return pacmanArch
}
