package constants

import (
	"strings"
	"testing"
)

func TestConstants(t *testing.T) {
	// Test color constants
	if ColorYellow == "" {
		t.Error("ColorYellow constant is empty")
	}

	if ColorBlue == "" {
		t.Error("ColorBlue constant is empty")
	}

	if ColorWhite == "" {
		t.Error("ColorWhite constant is empty")
	}

	// Test URL constants
	if DockerOrg == "" {
		t.Error("DockerOrg constant is empty")
	}

	if !strings.Contains(DockerOrg, "docker.io") {
		t.Error("DockerOrg should contain docker.io")
	}

	if Git != "git" {
		t.Errorf("Git constant should be 'git', got '%s'", Git)
	}

	if GoArchiveURL == "" {
		t.Error("GoArchiveURL constant is empty")
	}

	if !strings.Contains(GoArchiveURL, "https://") {
		t.Error("GoArchiveURL should be a valid HTTPS URL")
	}

	if YAPVersion == "" {
		t.Error("YAPVersion constant is empty")
	}

	if !strings.HasPrefix(YAPVersion, "v") {
		t.Error("YAPVersion should start with 'v'")
	}
}

func TestReleases(t *testing.T) {
	expectedReleases := []string{
		"almalinux", "alpine", "amzn", "arch", "centos",
		"debian", "fedora", "linuxmint", "opensuse-leap",
		"ol", "pop", "rhel", "rocky", "ubuntu",
	}

	if len(Releases) == 0 {
		t.Fatal("Releases array is empty")
	}

	// Check that all expected releases are present
	releaseMap := make(map[string]bool)
	for _, release := range Releases {
		releaseMap[release] = true
	}

	for _, expected := range expectedReleases {
		if !releaseMap[expected] {
			t.Errorf("Expected release '%s' not found in Releases", expected)
		}
	}

	// Verify no duplicates
	if len(releaseMap) != len(Releases) {
		t.Error("Releases array contains duplicates")
	}
}

func TestDistroToPackageManager(t *testing.T) {
	// Test that all known distros have package managers
	expectedMappings := map[string]string{
		"almalinux":     "yum",
		"alpine":        "apk",
		"amzn":          "yum",
		"arch":          "pacman",
		"centos":        "yum",
		"debian":        "apt",
		"fedora":        "yum",
		"linuxmint":     "apt",
		"ol":            "yum",
		"opensuse-leap": "zypper",
		"pop":           "apt",
		"rhel":          "yum",
		"rocky":         "yum",
		"ubuntu":        "apt",
	}

	if len(DistroToPackageManager) == 0 {
		t.Fatal("DistroToPackageManager map is empty")
	}

	for distro, expectedPkg := range expectedMappings {
		if actualPkg, exists := DistroToPackageManager[distro]; !exists {
			t.Errorf("Distro '%s' not found in DistroToPackageManager", distro)
		} else if actualPkg != expectedPkg {
			t.Errorf("Distro '%s': expected package manager '%s', got '%s'", distro, expectedPkg, actualPkg)
		}
	}

	// Test that all package managers are valid
	validPackagers := map[string]bool{
		"apk": true, "apt": true, "pacman": true, "yum": true, "zypper": true,
	}

	for distro, pkgManager := range DistroToPackageManager {
		if !validPackagers[pkgManager] {
			t.Errorf("Distro '%s' has invalid package manager '%s'", distro, pkgManager)
		}
	}
}

func TestPackers(t *testing.T) {
	expectedPackers := []string{"apk", "apt", "pacman", "yum", "zypper"}

	if len(Packers) == 0 {
		t.Fatal("Packers array is empty")
	}

	// Check that all expected packers are present
	packerMap := make(map[string]bool)
	for _, packer := range Packers {
		packerMap[packer] = true
	}

	for _, expected := range expectedPackers {
		if !packerMap[expected] {
			t.Errorf("Expected packer '%s' not found in Packers", expected)
		}
	}

	// Verify no duplicates
	if len(packerMap) != len(Packers) {
		t.Error("Packers array contains duplicates")
	}

	// Verify length matches expected
	if len(Packers) != len(expectedPackers) {
		t.Errorf("Expected %d packers, got %d", len(expectedPackers), len(Packers))
	}
}

func TestInitialization(t *testing.T) {
	// Test that initialization populated the global variables correctly

	// Test Distros slice
	if len(Distros) == 0 {
		t.Error("Distros slice not populated during initialization")
	}

	// Test DistrosSet
	if DistrosSet == nil {
		t.Fatal("DistrosSet is nil")
	}

	// Verify DistrosSet contains expected distros
	for _, distro := range Distros {
		if !DistrosSet.Contains(distro) {
			t.Errorf("DistrosSet does not contain distro '%s'", distro)
		}
	}

	// Test DistroPackageManager
	if len(DistroPackageManager) == 0 {
		t.Error("DistroPackageManager not populated during initialization")
	}

	// Verify DistroPackageManager matches expected mappings
	for _, distro := range Distros {
		expectedPkgMgr := DistroToPackageManager[distro]
		actualPkgMgr := DistroPackageManager[distro]

		if actualPkgMgr != expectedPkgMgr {
			t.Errorf("DistroPackageManager['%s']: expected '%s', got '%s'",
				distro, expectedPkgMgr, actualPkgMgr)
		}
	}

	// Test PackagersSet
	if PackagersSet == nil {
		t.Fatal("PackagersSet is nil")
	}

	// Verify PackagersSet contains all packers
	for _, packer := range Packers {
		if !PackagersSet.Contains(packer) {
			t.Errorf("PackagersSet does not contain packer '%s'", packer)
		}
	}
}

func TestDistroExtraction(t *testing.T) {
	// Test that distro names are correctly extracted from releases
	// (checking the init() function logic indirectly)
	for _, release := range Releases {
		distro := strings.Split(release, "_")[0]

		// Verify this distro exists in our maps
		if _, exists := DistroToPackageManager[distro]; !exists {
			t.Errorf("Distro '%s' extracted from release '%s' not found in DistroToPackageManager",
				distro, release)
		}

		if !DistrosSet.Contains(distro) {
			t.Errorf("Distro '%s' extracted from release '%s' not found in DistrosSet",
				distro, release)
		}
	}
}

func TestCleanPreviousFlag(t *testing.T) {
	// Test that CleanPrevious is initialized to false
	if CleanPrevious != false {
		t.Error("CleanPrevious should be initialized to false")
	}
}

func TestConsistency(t *testing.T) {
	// Test consistency between different data structures

	// All distros in DistroToPackageManager should be in releases
	releaseDistros := make(map[string]bool)

	for _, release := range Releases {
		distro := strings.Split(release, "_")[0]
		releaseDistros[distro] = true
	}

	for distro := range DistroToPackageManager {
		if !releaseDistros[distro] {
			t.Errorf("Distro '%s' in DistroToPackageManager but not in releases", distro)
		}
	}

	// All package managers in DistroToPackageManager should be in Packers
	packerSet := make(map[string]bool)
	for _, packer := range Packers {
		packerSet[packer] = true
	}

	for distro, pkgMgr := range DistroToPackageManager {
		if !packerSet[pkgMgr] {
			t.Errorf("Package manager '%s' for distro '%s' not found in Packers", pkgMgr, distro)
		}
	}
}
