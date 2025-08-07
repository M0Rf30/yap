package osutils

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

// OSRelease represents the contents of the /etc/os-release file.
type OSRelease struct {
	ID string
}

// ParseOSRelease reads the /etc/os-release file and populates the OSRelease struct.
func ParseOSRelease() (OSRelease, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return OSRelease{}, err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			Logger.Warn("failed to close os-release file", Logger.Args("error", err))
		}
	}()

	var osRelease OSRelease

	scanner := bufio.NewScanner(file)

	// Map to associate keys with struct fields
	fieldMap := map[string]*string{
		"ID": &osRelease.ID,
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)

		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(parts[1], "\"")

			if fieldPtr, ok := fieldMap[key]; ok {
				*fieldPtr = value
			}
		}
	}

	err = scanner.Err()
	if err != nil {
		return OSRelease{}, err
	}

	return osRelease, nil
}

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
