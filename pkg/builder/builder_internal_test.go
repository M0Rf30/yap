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
