package builder

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestProcessFunctionImplicitSrcdirCd asserts makepkg parity: every PKGBUILD
// function (prepare/build/check/package) starts with an implicit
// `cd "${srcdir}"`, so function bodies may use relative paths from srcdir.
func TestProcessFunctionImplicitSrcdirCd(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "cwd.txt")

	b := &Builder{
		PKGBUILD: &pkgbuild.PKGBUILD{
			PkgName:   "yap-srcdir-test",
			PkgVer:    "1.0.0",
			PkgRel:    "1",
			SourceDir: srcDir,
		},
	}

	body := "pwd > " + strconv.Quote(outFile)
	if err := b.processFunction(
		context.Background(), body, "logger.preparing_sources", "prepare",
	); err != nil {
		t.Fatalf("processFunction failed: %v", err)
	}

	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("function body did not run: %v", err)
	}

	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("EvalSymlinks(got): %v", err)
	}

	want, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(srcDir): %v", err)
	}

	if got != want {
		t.Fatalf("function ran in %q, want srcdir %q", got, want)
	}
}

// TestRunBuildStagesCheckStage asserts that check() runs between build and
// package, starts in "${srcdir}" (makepkg parity), and is skipped entirely
// when NoCheck is set (makepkg --nocheck).
func TestRunBuildStagesCheckStage(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, noCheck bool) (srcDir, checkOut string, err error) {
		t.Helper()

		srcDir = t.TempDir()
		checkOut = filepath.Join(t.TempDir(), "check-cwd.txt")

		b := &Builder{
			NoCheck: noCheck,
			PKGBUILD: &pkgbuild.PKGBUILD{
				PkgName:   "yap-check-test",
				PkgVer:    "1.0.0",
				PkgRel:    "1",
				SourceDir: srcDir,
				Build:     "true",
				Check:     "pwd > " + strconv.Quote(checkOut),
			},
		}

		return srcDir, checkOut, b.runBuildStages(context.Background())
	}

	t.Run("check runs in srcdir", func(t *testing.T) {
		t.Parallel()

		srcDir, outFile, err := run(t, false)
		if err != nil {
			t.Fatalf("runBuildStages failed: %v", err)
		}

		raw, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("check() did not run: %v", err)
		}

		got, err := filepath.EvalSymlinks(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("EvalSymlinks(got): %v", err)
		}

		want, err := filepath.EvalSymlinks(srcDir)
		if err != nil {
			t.Fatalf("EvalSymlinks(srcDir): %v", err)
		}

		if got != want {
			t.Fatalf("check() ran in %q, want srcdir %q", got, want)
		}
	})

	t.Run("nocheck skips check", func(t *testing.T) {
		t.Parallel()

		_, outFile, err := run(t, true)
		if err != nil {
			t.Fatalf("runBuildStages failed: %v", err)
		}

		if _, err := os.Stat(outFile); !os.IsNotExist(err) {
			t.Fatalf("check() ran despite NoCheck=true (stat err: %v)", err)
		}
	})
}
