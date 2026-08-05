package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

// resetConvertFlags restores every convert flag global to its zero value so
// tests don't leak state into one another.
func resetConvertFlags() {
	convertOutput = ""
	convertTo = ""
	convertPackager = ""
	convertContentsFrom = ""
	convertForce = false
}

const testPKGBUILD = `pkgname=hello
pkgver=1.2.3
pkgrel=2
pkgdesc="A test package"
maintainer="Test Maintainer <test@example.com>"
url="https://example.com/hello"
arch=('x86_64')
license=('MIT')
depends=('glibc')

package() {
    install -Dm644 /dev/null "${pkgdir}/usr/share/doc/hello/README"
}
`

const testNfpmSpec = `name: hello
version: 1.2.3
release: "2"
description: A test package
maintainer: Test Maintainer <test@example.com>
homepage: https://example.com/hello
license: MIT
arch: amd64
depends:
  - glibc
`

const testNfpmSpecWithOverride = testNfpmSpec + `overrides:
  deb:
    depends:
      - libc6
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestConvertCommandDefinition(t *testing.T) {
	assert.Equal(t, "convert <spec>", convertCmd.Use)
	assert.Equal(t, commandUtility, convertCmd.GroupID)
	assert.NotNil(t, convertCmd.RunE)
}

func TestInitializeConvertDescriptions(t *testing.T) {
	InitializeConvertDescriptions()
	assert.NotEmpty(t, convertCmd.Short,
		"convertCmd.Short should be non-empty after InitializeConvertDescriptions")
	assert.NotEmpty(t, convertCmd.Long)
	assert.NotEmpty(t, convertCmd.Example)
}

func TestDetectDialect(t *testing.T) {
	_ = i18n.Init("en")

	tests := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{name: "nfpm.yaml", path: "/tmp/x/nfpm.yaml", expected: dialectNFPM},
		{name: "nfpm.yml", path: "/tmp/x/nfpm.yml", expected: dialectNFPM},
		{name: "dot nfpm.yaml", path: "/tmp/x/.nfpm.yaml", expected: dialectNFPM},
		{name: "dot nfpm.yml", path: "/tmp/x/.nfpm.yml", expected: dialectNFPM},
		{name: "PKGBUILD", path: "/tmp/x/PKGBUILD", expected: dialectPKGBUILD},
		{name: "unrecognized", path: "/tmp/x/spec.txt", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectDialect(tt.path)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "PKGBUILD")
				assert.Contains(t, err.Error(), "nfpm.yaml")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestResolveSpecInput(t *testing.T) {
	_ = i18n.Init("en")

	t.Run("file input returned as-is", func(t *testing.T) {
		dir := t.TempDir()
		specPath := filepath.Join(dir, "nfpm.yaml")
		writeFile(t, specPath, testNfpmSpec)

		got, err := resolveSpecInput(specPath)
		require.NoError(t, err)
		assert.Equal(t, specPath, got)
	})

	t.Run("directory with PKGBUILD", func(t *testing.T) {
		dir := t.TempDir()
		pkgbuildPath := filepath.Join(dir, "PKGBUILD")
		writeFile(t, pkgbuildPath, testPKGBUILD)

		got, err := resolveSpecInput(dir)
		require.NoError(t, err)
		assert.Equal(t, pkgbuildPath, got)
	})

	t.Run("directory with nfpm spec", func(t *testing.T) {
		dir := t.TempDir()
		specPath := filepath.Join(dir, "nfpm.yaml")
		writeFile(t, specPath, testNfpmSpec)

		got, err := resolveSpecInput(dir)
		require.NoError(t, err)
		assert.Equal(t, specPath, got)
	})

	t.Run("directory with PKGBUILD wins over nfpm spec", func(t *testing.T) {
		dir := t.TempDir()
		pkgbuildPath := filepath.Join(dir, "PKGBUILD")
		writeFile(t, pkgbuildPath, testPKGBUILD)
		writeFile(t, filepath.Join(dir, "nfpm.yaml"), testNfpmSpec)

		got, err := resolveSpecInput(dir)
		require.NoError(t, err)
		assert.Equal(t, pkgbuildPath, got)
	})

	t.Run("directory with no spec", func(t *testing.T) {
		dir := t.TempDir()

		_, err := resolveSpecInput(dir)
		require.Error(t, err)
	})

	t.Run("nonexistent path", func(t *testing.T) {
		_, err := resolveSpecInput(filepath.Join(t.TempDir(), "does-not-exist"))
		require.Error(t, err)
	})
}

func TestRunConvertCommand_NfpmDirectoryInputDefaultsToPKGBUILD(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "nfpm.yaml"), testNfpmSpec)

	resetConvertFlags()
	t.Cleanup(resetConvertFlags)

	err := runConvertCommand(convertCmd, []string{dir})
	require.NoError(t, err)

	outputPath := filepath.Join(dir, "PKGBUILD")
	require.FileExists(t, outputPath)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(content), "hello")
}

func TestRunConvertCommand_StdoutOutput(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "nfpm.yaml")
	writeFile(t, specPath, testNfpmSpec)

	resetConvertFlags()

	convertOutput = "-"

	t.Cleanup(resetConvertFlags)

	var out bytes.Buffer

	convertCmd.SetOut(&out)
	t.Cleanup(func() { convertCmd.SetOut(nil) })

	err := runConvertCommand(convertCmd, []string{specPath})
	require.NoError(t, err)

	assert.Contains(t, out.String(), "hello")
	assert.NoFileExists(t, filepath.Join(dir, "PKGBUILD"))
}

func TestRunConvertCommand_RefuseClobberWithoutForce(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "nfpm.yaml")
	writeFile(t, specPath, testNfpmSpec)

	outputPath := filepath.Join(dir, "PKGBUILD")
	writeFile(t, outputPath, "# pre-existing\n")

	resetConvertFlags()
	t.Cleanup(resetConvertFlags)

	err := runConvertCommand(convertCmd, []string{specPath})
	require.Error(t, err)

	before, readErr := os.ReadFile(outputPath) //nolint:gosec // test-controlled path
	require.NoError(t, readErr)
	assert.Equal(t, "# pre-existing\n", string(before), "output must not be overwritten without -f")

	convertForce = true

	err = runConvertCommand(convertCmd, []string{specPath})
	require.NoError(t, err)

	after, readErr := os.ReadFile(outputPath) //nolint:gosec // test-controlled path
	require.NoError(t, readErr)
	assert.NotEqual(t, "# pre-existing\n", string(after), "output must be overwritten with -f")
}

func TestRunConvertCommand_ToFlag(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	pkgbuildPath := filepath.Join(dir, "PKGBUILD")
	writeFile(t, pkgbuildPath, testPKGBUILD)

	t.Run("explicit --to matching the default opposite dialect succeeds", func(t *testing.T) {
		resetConvertFlags()
		t.Cleanup(resetConvertFlags)

		convertTo = dialectNFPM
		convertOutput = "-"

		var out bytes.Buffer

		convertCmd.SetOut(&out)
		t.Cleanup(func() { convertCmd.SetOut(nil) })

		err := runConvertCommand(convertCmd, []string{pkgbuildPath})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "hello")
	})

	t.Run("invalid --to value is rejected", func(t *testing.T) {
		resetConvertFlags()
		t.Cleanup(resetConvertFlags)

		convertTo = "rpm"

		err := runConvertCommand(convertCmd, []string{pkgbuildPath})
		require.Error(t, err)
	})

	t.Run("--to matching the input dialect is rejected", func(t *testing.T) {
		resetConvertFlags()
		t.Cleanup(resetConvertFlags)

		convertTo = dialectPKGBUILD

		err := runConvertCommand(convertCmd, []string{pkgbuildPath})
		require.Error(t, err)
	})
}

func TestRunConvertCommand_PackagerCollapsing(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "nfpm.yaml")
	writeFile(t, specPath, testNfpmSpecWithOverride)

	t.Run("without --packager the suffixed override is preserved", func(t *testing.T) {
		resetConvertFlags()
		t.Cleanup(resetConvertFlags)

		convertOutput = "-"

		var out bytes.Buffer

		convertCmd.SetOut(&out)
		t.Cleanup(func() { convertCmd.SetOut(nil) })

		err := runConvertCommand(convertCmd, []string{specPath})
		require.NoError(t, err)
		assert.Contains(t, out.String(), "__apt")
		assert.Contains(t, out.String(), "glibc")
	})

	t.Run("with --packager deb the config is collapsed", func(t *testing.T) {
		resetConvertFlags()
		t.Cleanup(resetConvertFlags)

		convertOutput = "-"
		convertPackager = nfpm.PackagerDeb

		var out bytes.Buffer

		convertCmd.SetOut(&out)
		t.Cleanup(func() { convertCmd.SetOut(nil) })

		err := runConvertCommand(convertCmd, []string{specPath})
		require.NoError(t, err)
		assert.NotContains(t, out.String(), "__apt")
		assert.Contains(t, out.String(), "libc6")
	})
}

func TestRunConvertCommand_PKGBUILDToNfpmReparse(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	pkgbuildPath := filepath.Join(dir, "PKGBUILD")
	writeFile(t, pkgbuildPath, testPKGBUILD)

	resetConvertFlags()
	t.Cleanup(resetConvertFlags)

	err := runConvertCommand(convertCmd, []string{pkgbuildPath})
	require.NoError(t, err)

	outputPath := filepath.Join(dir, "nfpm.yaml")
	require.FileExists(t, outputPath)

	cfg, err := nfpm.Load(outputPath)
	require.NoError(t, err)

	assert.Equal(t, "hello", cfg.Name)
	assert.Equal(t, "1.2.3", cfg.Version)
	assert.Equal(t, "2", cfg.Release)
	assert.Equal(t, "A test package", cfg.Description)
	assert.Equal(t, "MIT", cfg.License)
	assert.Contains(t, cfg.Maintainer, "Test Maintainer")
	assert.Contains(t, cfg.Depends, "glibc")
}

func TestRunConvertCommand_NfpmToPKGBUILDExactText(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "nfpm.yaml")
	writeFile(t, specPath, testNfpmSpec)

	resetConvertFlags()
	t.Cleanup(resetConvertFlags)

	convertOutput = "-"

	var out bytes.Buffer

	convertCmd.SetOut(&out)
	t.Cleanup(func() { convertCmd.SetOut(nil) })

	err := runConvertCommand(convertCmd, []string{specPath})
	require.NoError(t, err)

	const expected = "pkgname=\"hello\"\n" +
		"pkgbase=\"hello\"\n" +
		"pkgver=\"1.2.3\"\n" +
		"pkgrel=\"2\"\n" +
		"pkgdesc=\"A test package\"\n" +
		"maintainer=\"Test Maintainer <test@example.com>\"\n" +
		"url=\"https://example.com/hello\"\n" +
		"priority=\"optional\"\n" +
		"arch=('x86_64')\n" +
		"license=('MIT')\n" +
		"depends=('glibc')\n" +
		"package() {\n" +
		"}\n"

	assert.Equal(t, expected, out.String())
}

const testChangelogYAML = `name: hello
entries:
  - semver: "1.2.3-2"
    date: 2025-01-01T00:00:00Z
    changes:
      - note: "Initial release"
`

// TestRunConvertCommand_NfpmChangelogWritesSidecar covers the changelog:
// field: converting an nfpm spec with a changelog to a PKGBUILD file
// writes the RPM-dialect "<name>.changelog" sidecar beside it and the
// PKGBUILD carries the matching changelog= directive.
func TestRunConvertCommand_NfpmChangelogWritesSidecar(t *testing.T) {
	_ = i18n.Init("en")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "nfpm.yaml")
	writeFile(t, specPath, testNfpmSpec+"changelog: changelog.yaml\n")
	writeFile(t, filepath.Join(dir, "changelog.yaml"), testChangelogYAML)

	resetConvertFlags()
	t.Cleanup(resetConvertFlags)

	err := runConvertCommand(convertCmd, []string{specPath})
	require.NoError(t, err)

	outputPath := filepath.Join(dir, "PKGBUILD")
	require.FileExists(t, outputPath)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(content), `changelog="hello.changelog"`)

	sidecarPath := filepath.Join(dir, "hello.changelog")
	require.FileExists(t, sidecarPath)

	sidecar, err := os.ReadFile(sidecarPath) //nolint:gosec // test-controlled path
	require.NoError(t, err)
	assert.Contains(t, string(sidecar), "Initial release")
}
