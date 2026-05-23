package dnfinstall //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/M0Rf30/rpmpack"
	rpmutils "github.com/sassoftware/go-rpmutils"
	"github.com/stretchr/testify/require"
)

// buildRPMWithCapabilities creates an RPM with Provides, Requires, Conflicts, and Obsoletes.
// The package name is fixed to "test-pkg" since that's all callers use.
//
//nolint:unparam // version/release kept for documentation / future expansion
func buildRPMWithCapabilities(t *testing.T, tmpDir, _, version, release string) string {
	t.Helper()

	const name = "test-pkg"

	provides := rpmpack.Relations{}
	requires := rpmpack.Relations{}
	conflicts := rpmpack.Relations{}
	obsoletes := rpmpack.Relations{}

	// Set capabilities using the Set method.
	_ = provides.Set(name + "-lib")
	_ = provides.Set("virtual-lib")
	_ = requires.Set("libc.so.6")
	_ = requires.Set("/bin/sh")
	_ = conflicts.Set("old-pkg")
	_ = obsoletes.Set("legacy-pkg")

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       name,
		Version:    version,
		Release:    release,
		Arch:       "x86_64",
		Summary:    "Test package with capabilities",
		Compressor: "gzip",
		BuildTime:  time.Now(),
		Provides:   provides,
		Requires:   requires,
		Conflicts:  conflicts,
		Obsoletes:  obsoletes,
	})
	require.NoError(t, err)

	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/usr/bin/test",
		Body:  []byte("#!/bin/sh\necho test\n"),
		Mode:  0o755,
		MTime: uint32(time.Now().Unix()),
	})

	rpmPath := filepath.Join(tmpDir, name+"-"+version+"-"+release+".x86_64.rpm")
	f, err := os.Create(rpmPath)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	err = rpm.Write(f)
	require.NoError(t, err)

	return rpmPath
}

// readRPMHeader reads an RPM file and returns the parsed header.
func readRPMHeader(t *testing.T, rpmPath string) *rpmutils.Rpm {
	t.Helper()

	f, err := os.Open(rpmPath)
	require.NoError(t, err, "failed to open RPM")

	defer func() { _ = f.Close() }()

	rpm, err := rpmutils.ReadRpm(f)
	require.NoError(t, err, "failed to parse RPM")

	return rpm
}
