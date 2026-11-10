// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgrpm "github.com/M0Rf30/pkgformat/rpm"
	"github.com/M0Rf30/pkgformat/rpm/pack"
	"github.com/stretchr/testify/require"
)

// buildRPMWithCapabilities creates an RPM with Provides, Requires, Conflicts, and Obsoletes.
// The package name is fixed to "test-pkg" since that's all callers use.
//
//nolint:unparam // version/release kept for documentation / future expansion
func buildRPMWithCapabilities(t *testing.T, tmpDir, _, version, release string) string {
	t.Helper()

	const name = "test-pkg"

	provides := pack.Relations{}
	requires := pack.Relations{}
	conflicts := pack.Relations{}
	obsoletes := pack.Relations{}

	// Set capabilities using the Set method.
	_ = provides.Set(name + "-lib")
	_ = provides.Set("virtual-lib")
	_ = requires.Set("libc.so.6")
	_ = requires.Set("/bin/sh")
	_ = conflicts.Set("old-pkg")
	_ = obsoletes.Set("legacy-pkg")

	rpm, err := pack.NewRPM(pack.RPMMetaData{
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

	rpm.AddFile(pack.RPMFile{
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
func readRPMHeader(t *testing.T, rpmPath string) *pkgrpm.Rpm {
	t.Helper()

	f, err := os.Open(rpmPath)
	require.NoError(t, err, "failed to open RPM")

	defer func() { _ = f.Close() }()

	rpm, err := pkgrpm.ReadRpm(f)
	require.NoError(t, err, "failed to parse RPM")

	return rpm
}
