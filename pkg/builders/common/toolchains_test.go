package common

import (
	"testing"
)

func TestGetExecutableName(t *testing.T) {
	tests := []struct {
		name         string
		packageName  string
		expectedExec string
		description  string
	}{
		{
			name:         "Debian/Ubuntu GCC",
			packageName:  "gcc-aarch64-linux-gnu",
			expectedExec: "aarch64-linux-gnu-gcc",
			description:  "Debian/Ubuntu package name to executable",
		},
		{
			name:         "Debian/Ubuntu G++",
			packageName:  "g++-aarch64-linux-gnu",
			expectedExec: "aarch64-linux-gnu-g++",
			description:  "Debian/Ubuntu G++ package name to executable",
		},
		{
			name:         "Fedora GCC",
			packageName:  "gcc-aarch64-linux-gnu",
			expectedExec: "aarch64-linux-gnu-gcc",
			description:  "Fedora package name to executable",
		},
		{
			name:         "Fedora G++",
			packageName:  "gcc-c++-aarch64-linux-gnu",
			expectedExec: "aarch64-linux-gnu-g++",
			description:  "Fedora G++ (gcc-c++) package name to executable",
		},
		{
			name:         "Arch Linux GCC",
			packageName:  "aarch64-linux-gnu-gcc",
			expectedExec: "aarch64-linux-gnu-gcc",
			description:  "Arch Linux package name (already correct format)",
		},
		{
			name:         "Arch Linux G++",
			packageName:  "aarch64-linux-gnu-g++",
			expectedExec: "aarch64-linux-gnu-g++",
			description:  "Arch Linux G++ package name (already correct format)",
		},
		{
			name:         "Alpine GCC",
			packageName:  "gcc-aarch64",
			expectedExec: "aarch64-gcc",
			description:  "Alpine package name to executable (basic transform)",
		},
		{
			name:         "Alpine G++",
			packageName:  "g++-armv7",
			expectedExec: "armv7-g++",
			description:  "Alpine G++ package name to executable",
		},
		{
			name:         "ARM v7 Debian",
			packageName:  "gcc-arm-linux-gnueabihf",
			expectedExec: "arm-linux-gnueabihf-gcc",
			description:  "ARM v7 Debian package",
		},
		{
			name:         "ARM v7 G++ Debian",
			packageName:  "g++-arm-linux-gnueabihf",
			expectedExec: "arm-linux-gnueabihf-g++",
			description:  "ARM v7 G++ Debian package",
		},
		{
			name:         "i686 Debian",
			packageName:  "gcc-i686-linux-gnu",
			expectedExec: "i686-linux-gnu-gcc",
			description:  "i686 Debian package",
		},
		{
			name:         "PowerPC64LE Debian",
			packageName:  "gcc-powerpc64le-linux-gnu",
			expectedExec: "powerpc64le-linux-gnu-gcc",
			description:  "PowerPC64LE Debian package",
		},
		{
			name:         "s390x Fedora",
			packageName:  "gcc-s390x-redhat-linux",
			expectedExec: "s390x-redhat-linux-gcc",
			description:  "s390x Fedora package",
		},
		{
			name:         "RISC-V 64 Debian",
			packageName:  "gcc-riscv64-linux-gnu",
			expectedExec: "riscv64-linux-gnu-gcc",
			description:  "RISC-V 64-bit Debian package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := CrossToolchain{
				GCCPackage: tt.packageName,
			}

			result := ct.GetExecutableName(tt.packageName)

			if result != tt.expectedExec {
				t.Errorf("%s: got %q, want %q", tt.description, result, tt.expectedExec)
			}

			t.Logf("%s: %s -> %s ✓", tt.name, tt.packageName, result)
		})
	}
}

func TestGetExecutableNameAllDistributions(t *testing.T) {
	// Test all architectures across all distributions
	architectures := []string{"aarch64", "armv7", "armv6", "i686", "x86_64", "ppc64le", "s390x", "riscv64"}
	distributions := []string{"debian", "ubuntu", "fedora", "alpine", "arch"}

	for _, arch := range architectures {
		for _, distro := range distributions {
			t.Run(arch+"_"+distro, func(t *testing.T) {
				toolchainMap, exists := CrossToolchainMap[arch]
				if !exists {
					t.Skipf("Architecture %s not in CrossToolchainMap", arch)
					return
				}

				toolchain, exists := toolchainMap[distro]
				if !exists {
					t.Skipf("Distribution %s not supported for architecture %s", distro, arch)
					return
				}

				gccExec := toolchain.GetExecutableName(toolchain.GCCPackage)
				gppExec := toolchain.GetExecutableName(toolchain.GPlusPlusPackage)

				// Verify no package names leak into executables
				// Package names typically start with gcc- or g++-
				if distro == "debian" || distro == "ubuntu" || distro == "fedora" {
					// For these distros, executables should NOT start with gcc- or g++-
					// They should be like: aarch64-linux-gnu-gcc
					if len(gccExec) > 4 && gccExec[:4] == "gcc-" && gccExec != "gcc-multilib" {
						t.Errorf("GCC executable still has package format: %s (package: %s)",
							gccExec, toolchain.GCCPackage)
					}

					if len(gppExec) > 4 && gppExec[:4] == "g++-" {
						t.Errorf("G++ executable still has package format: %s (package: %s)",
							gppExec, toolchain.GPlusPlusPackage)
					}
				}

				t.Logf("%s on %s:", arch, distro)
				t.Logf("  GCC Package: %s -> Executable: %s", toolchain.GCCPackage, gccExec)
				t.Logf("  G++ Package: %s -> Executable: %s", toolchain.GPlusPlusPackage, gppExec)
			})
		}
	}
}
