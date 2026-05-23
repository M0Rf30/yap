package dnfinstall

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/M0Rf30/rpmpack"
	rpmutils "github.com/sassoftware/go-rpmutils"
	"github.com/stretchr/testify/require"
)

// buildMinimalRPM creates a minimal valid RPM file using rpmpack.
// Returns the path to the created RPM file.
func buildMinimalRPM(t *testing.T, tmpDir string, name, version, release string) string {
	t.Helper()

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       name,
		Version:    version,
		Release:    release,
		Arch:       "x86_64",
		Summary:    "Test package",
		Compressor: "gzip",
		BuildTime:  time.Now(),
	})
	require.NoError(t, err, "failed to create RPM metadata")

	// Add a simple file to the RPM.
	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/usr/bin/test",
		Body:  []byte("#!/bin/sh\necho test\n"),
		Mode:  0o755,
		MTime: uint32(time.Now().Unix()),
	})

	rpmPath := filepath.Join(tmpDir, name+"-"+version+"-"+release+".x86_64.rpm")
	f, err := os.Create(rpmPath)
	require.NoError(t, err, "failed to create RPM file")
	defer func() { _ = f.Close() }()

	err = rpm.Write(f)
	require.NoError(t, err, "failed to write RPM")

	return rpmPath
}

// buildRPMWithScriptlet creates an RPM with a scriptlet.
// scriptletKind should be one of: "prein", "postin", "pretrans", "posttrans"
// scriptletBody is the shell script to execute.
func buildRPMWithScriptlet(t *testing.T, tmpDir string, name, version, release, scriptletKind, scriptletBody string) string {
	t.Helper()

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       name,
		Version:    version,
		Release:    release,
		Arch:       "x86_64",
		Summary:    "Test package with scriptlet",
		Compressor: "gzip",
		BuildTime:  time.Now(),
	})
	require.NoError(t, err)

	// Add a simple file.
	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/usr/bin/test",
		Body:  []byte("#!/bin/sh\necho test\n"),
		Mode:  0o755,
		MTime: uint32(time.Now().Unix()),
	})

	// Add scriptlet based on kind.
	switch scriptletKind {
	case "prein":
		rpm.AddPrein(scriptletBody)
	case "postin":
		rpm.AddPostin(scriptletBody)
	case "pretrans":
		rpm.AddPretrans(scriptletBody)
	case "posttrans":
		rpm.AddPosttrans(scriptletBody)
	default:
		t.Fatalf("unknown scriptlet kind: %s", scriptletKind)
	}

	rpmPath := filepath.Join(tmpDir, name+"-"+version+"-"+release+".x86_64.rpm")
	f, err := os.Create(rpmPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	err = rpm.Write(f)
	require.NoError(t, err)

	return rpmPath
}

// buildRPMWithCapabilities creates an RPM with Provides, Requires, Conflicts, and Obsoletes.
func buildRPMWithCapabilities(t *testing.T, tmpDir string, name, version, release string) string {
	t.Helper()

	provides := rpmpack.Relations{}
	requires := rpmpack.Relations{}
	conflicts := rpmpack.Relations{}
	obsoletes := rpmpack.Relations{}

	// Set capabilities using the Set method.
	_ = provides.Set(name + "-lib")
	_ = provides.Set("virtual-lib")
	_ = requires.Set("libc.so.6")
	_ = requires.Set("bash")
	_ = conflicts.Set("oldpkg")
	_ = obsoletes.Set("deprecated-pkg")

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
		Name:  "/usr/lib/libtest.so",
		Body:  []byte("fake library"),
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

// buildRPMWithConfigFile creates an RPM with a %config(noreplace) file.
// Note: rpmpack may not support %config flags directly, so we'll create a basic RPM
// and the test will verify the file extraction behavior.
func buildRPMWithConfigFile(t *testing.T, tmpDir string, name, version, release string) string {
	t.Helper()

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       name,
		Version:    version,
		Release:    release,
		Arch:       "x86_64",
		Summary:    "Test package with config file",
		Compressor: "gzip",
		BuildTime:  time.Now(),
	})
	require.NoError(t, err)

	// Add a config file.
	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/etc/test.conf",
		Body:  []byte("# Test config\nkey=value\n"),
		Mode:  0o644,
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

// buildRPMWithSymlink creates an RPM with a symlink.
func buildRPMWithSymlink(t *testing.T, tmpDir string, name, version, release string) string {
	t.Helper()

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       name,
		Version:    version,
		Release:    release,
		Arch:       "x86_64",
		Summary:    "Test package with symlink",
		Compressor: "gzip",
		BuildTime:  time.Now(),
	})
	require.NoError(t, err)

	// Add a regular file.
	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/usr/bin/real-binary",
		Body:  []byte("#!/bin/sh\necho real\n"),
		Mode:  0o755,
		MTime: uint32(time.Now().Unix()),
	})

	// Add a symlink. In rpmpack, symlinks are created by setting Body to the target
	// and Mode to indicate a symlink (0o120000).
	rpm.AddFile(rpmpack.RPMFile{
		Name:  "/usr/bin/symlink-binary",
		Body:  []byte("real-binary"), // symlink target
		Mode:  0o120000,               // symlink mode
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

// extractRPMToBuffer extracts the payload of an RPM and returns it as bytes.
// This is useful for testing the extraction logic.
func extractRPMToBuffer(t *testing.T, rpmPath string) *bytes.Buffer {
	t.Helper()

	f, err := os.Open(rpmPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	rpm, err := rpmutils.ReadRpm(f)
	require.NoError(t, err)

	payload, err := rpm.PayloadReader()
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = buf.ReadFrom(payload)
	require.NoError(t, err)

	return &buf
}

// installRPMWithContext is a helper to install an RPM with a given context.
func installRPMWithContext(t *testing.T, ctx context.Context, rpmPath, rootDir string, opts Options) error {
	t.Helper()

	return installPackage(ctx, rpmPath, rootDir, opts)
}
