package rpm

import (
	"os"
	"path/filepath"
	"testing"

	rpmpack "github.com/M0Rf30/rpmpack"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/files"
)

func TestPosixMode(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want uint
	}{
		{"plain", 0o644, 0o644},
		{"setuid", 0o755 | os.ModeSetuid, 0o4755},
		{"setgid", 0o755 | os.ModeSetgid, 0o2755},
		{"sticky", 0o777 | os.ModeSticky, 0o1777},
		{"dir bit dropped", 0o755 | os.ModeDir, 0o755},
		{"all special", 0o755 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky, 0o7755},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := posixMode(tt.mode); got != tt.want {
				t.Errorf("posixMode(%v) = %#o, want %#o", tt.mode, got, tt.want)
			}
		})
	}
}

func TestAsRPMFilePreservesSetuid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "su")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0o755|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}

	f, err := asRPMFile(&files.Entry{Source: path, Destination: "/usr/bin/su"}, rpmpack.GenericFile)
	if err != nil {
		t.Fatal(err)
	}

	if f.Mode != 0o4755 {
		t.Errorf("Mode = %#o, want 04755", f.Mode)
	}
}

func TestAsRPMDirectoryPreservesSticky(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tmp")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}

	f, err := asRPMDirectory(&files.Entry{Source: dir, Destination: "/var/tmp"})
	if err != nil {
		t.Fatal(err)
	}

	if want := uint(0o1777 | files.TagDirectory); f.Mode != want {
		t.Errorf("Mode = %#o, want %#o", f.Mode, want)
	}
}

func TestGetGroupIdempotentForSplitPackages(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Section = "devel"
	r := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	for i := range 3 {
		r.getGroup()

		if pkgBuild.Section != tools {
			t.Fatalf("call %d: Section = %q, want %q", i, pkgBuild.Section, tools)
		}
	}
}

func TestPackageFileNameOmitsEpoch(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Epoch = "2"
	r := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	if got, want := r.packageFileName(), "test-package-1.0.0-1.x86_64.rpm"; got != want {
		t.Errorf("packageFileName() = %q, want %q", got, want)
	}
}
