package utils

import (
	"runtime"
)

// GetArchitecture returns the corresponding uname -m output for the current GOARCH.
func GetArchitecture() string {
	// Create a map of GOARCH values to uname -m outputs
	architectureMap := map[string]string{
		"amd64":   "x86_64",
		"386":     "i386",
		"arm":     "armv7l",  // Common for ARM 32-bit
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

	// Get the corresponding uname -m output from the map
	unameOutput := architectureMap[currentArch]

	return unameOutput
}
