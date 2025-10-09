package packer

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/builders/apk"
	"github.com/M0Rf30/yap/v2/pkg/builders/deb"
	"github.com/M0Rf30/yap/v2/pkg/builders/pacman"
	"github.com/M0Rf30/yap/v2/pkg/builders/rpm"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestGetPackageManager_APK(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}

	packer := GetPackageManager(pkgBuild, "alpine")

	if packer == nil {
		t.Fatal("GetPackageManager returned nil for alpine")
	}

	_, ok := packer.(*apk.Apk)
	if !ok {
		t.Error("GetPackageManager did not return apk.Apk for alpine")
	}
}

func TestGetPackageManager_DEB(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}

	debDistros := []string{"debian", "ubuntu"}

	for _, distro := range debDistros {
		t.Run(distro, func(t *testing.T) {
			packer := GetPackageManager(pkgBuild, distro)

			if packer == nil {
				t.Fatalf("GetPackageManager returned nil for %s", distro)
			}

			_, ok := packer.(*deb.Package)
			if !ok {
				t.Errorf("GetPackageManager did not return deb.Package for %s", distro)
			}
		})
	}
}

func TestGetPackageManager_PKG(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}

	packer := GetPackageManager(pkgBuild, "arch")

	if packer == nil {
		t.Fatal("GetPackageManager returned nil for arch")
	}

	_, ok := packer.(*pacman.Pkg)
	if !ok {
		t.Error("GetPackageManager did not return pacman.Pkg for arch")
	}
}

func TestGetPackageManager_RPM_YUM(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}

	yumDistros := []string{"fedora", "centos", "rhel", "almalinux", "rocky", "amzn", "ol"}

	for _, distro := range yumDistros {
		t.Run(distro, func(t *testing.T) {
			packer := GetPackageManager(pkgBuild, distro)

			if packer == nil {
				t.Fatalf("GetPackageManager returned nil for %s", distro)
			}

			_, ok := packer.(*rpm.RPM)
			if !ok {
				t.Errorf("GetPackageManager did not return rpm.RPM for %s", distro)
			}
		})
	}
}

func TestGetPackageManager_RPM_Zypper(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}

	packer := GetPackageManager(pkgBuild, "opensuse-leap")

	if packer == nil {
		t.Fatal("GetPackageManager returned nil for opensuse-leap")
	}

	_, ok := packer.(*rpm.RPM)
	if !ok {
		t.Error("GetPackageManager did not return rpm.RPM for opensuse-leap")
	}
}

func TestGetPackageManager_Integration(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "integration-test",
		PkgVer:  "2.0.0",
		PkgRel:  "1",
		PkgDesc: "Integration test package",
	}

	// Test all supported distros return valid packers
	testCases := map[string]any{
		"alpine":              (*apk.Apk)(nil),
		"debian":              (*deb.Package)(nil),
		"ubuntu":              (*deb.Package)(nil),
		"arch":                (*pacman.Pkg)(nil),
		"fedora":              (*rpm.RPM)(nil),
		"centos":              (*rpm.RPM)(nil),
		"opensuse-leap":       (*rpm.RPM)(nil),
		"opensuse-tumbleweed": (*rpm.RPM)(nil),
	}

	for distro, expectedType := range testCases {
		t.Run(distro, func(t *testing.T) {
			packer := GetPackageManager(pkgBuild, distro)

			if packer == nil {
				t.Fatalf("GetPackageManager returned nil for %s", distro)
			}

			// Check that the packer has the correct PKGBUILD reference
			switch p := packer.(type) {
			case *apk.Apk:
				if expectedType != (*apk.Apk)(nil) {
					t.Errorf("Expected different type for %s", distro)
				}

				if p.PKGBUILD != pkgBuild {
					t.Error("PKGBUILD reference not set correctly")
				}
			case *deb.Package:
				if expectedType != (*deb.Package)(nil) {
					t.Errorf("Expected different type for %s", distro)
				}

				if p.PKGBUILD != pkgBuild {
					t.Error("PKGBUILD reference not set correctly")
				}
			case *pacman.Pkg:
				if expectedType != (*pacman.Pkg)(nil) {
					t.Errorf("Expected different type for %s", distro)
				}

				if p.PKGBUILD != pkgBuild {
					t.Error("PKGBUILD reference not set correctly")
				}
			case *rpm.RPM:
				if expectedType != (*rpm.RPM)(nil) {
					t.Errorf("Expected different type for %s", distro)
				}

				if p.PKGBUILD != pkgBuild {
					t.Error("PKGBUILD reference not set correctly")
				}
			default:
				t.Errorf("Unknown packer type returned for %s", distro)
			}
		})
	}
}

func TestGetPackageManager_ValidatesInterface(t *testing.T) {
	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "interface-test",
		PkgVer:  "1.0.0",
	}

	distros := []string{"alpine", "debian", "arch", "fedora", "opensuse-leap", "opensuse-tumbleweed"}

	for _, distro := range distros {
		t.Run(distro, func(t *testing.T) {
			packer := GetPackageManager(pkgBuild, distro)

			if packer == nil {
				t.Fatalf("GetPackageManager returned nil for %s", distro)
			}

			// Verify it implements the Packer interface by checking all methods exist
			_ = packer

			// This compilation check ensures the returned types implement all interface methods:
			// - BuildPackage(output string) error
			// - Install(output string) error
			// - Prepare(depends []string) error
			// - PrepareEnvironment(flag bool) error
			// - PrepareFakeroot(output string) error
			// - Update() error
		})
	}
}
