package aptinstall_test

// aptinstall_extra_test.go covers functions that were at 0% coverage:
//
//   - resolveRootDir      (line 138 in aptinstall.go)
//   - currentInstalledVersion (line 243 — tested via the in-memory helper)
//   - runMaintainerScript (line 373 — tested via RunMaintainerScriptForPhaseForTesting)
//   - RefreshLDCache      (line 418 — exported, smoke-tested)
//
// resolveAndPrepare (line 157) and installPackage (line 270) require a live
// apt cache + network; they are integration-tested elsewhere and are not
// covered here.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// ---------------------------------------------------------------------------
// resolveRootDir
// ---------------------------------------------------------------------------

// TestResolveRootDirEmptyBecomesSlash verifies that an empty RootDir is
// normalised to "/" and that AllowRootInstall=true lets it through.
func TestResolveRootDirEmptyBecomesSlash(t *testing.T) {
	t.Parallel()

	got, err := aptinstall.ResolveRootDirForTesting("", true)
	require.NoError(t, err)
	assert.Equal(t, "/", got)
}

// TestResolveRootDirSlashAllowed verifies that an explicit "/" is accepted
// when AllowRootInstall is true.
func TestResolveRootDirSlashAllowed(t *testing.T) {
	t.Parallel()

	got, err := aptinstall.ResolveRootDirForTesting("/", true)
	require.NoError(t, err)
	assert.Equal(t, "/", got)
}

// TestResolveRootDirSlashRefusedWithoutFlag verifies that "/" is rejected
// when AllowRootInstall is false (safety guard for developer workstations).
func TestResolveRootDirSlashRefusedWithoutFlag(t *testing.T) {
	t.Parallel()

	_, err := aptinstall.ResolveRootDirForTesting("/", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AllowRootInstall")
}

// TestResolveRootDirEmptyRefusedWithoutFlag verifies that an empty RootDir
// (which normalises to "/") is also refused when AllowRootInstall is false.
func TestResolveRootDirEmptyRefusedWithoutFlag(t *testing.T) {
	t.Parallel()

	_, err := aptinstall.ResolveRootDirForTesting("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AllowRootInstall")
}

// TestResolveRootDirFakerootPath verifies that a non-root path is always
// accepted regardless of AllowRootInstall.
func TestResolveRootDirFakerootPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, err := aptinstall.ResolveRootDirForTesting(dir, false)
	require.NoError(t, err)
	assert.Equal(t, dir, got)
}

// TestResolveRootDirFakerootPathWithAllowFlag verifies that a non-root path
// is accepted even when AllowRootInstall is true.
func TestResolveRootDirFakerootPathWithAllowFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, err := aptinstall.ResolveRootDirForTesting(dir, true)
	require.NoError(t, err)
	assert.Equal(t, dir, got)
}

// ---------------------------------------------------------------------------
// currentInstalledVersion (via in-memory helper)
// ---------------------------------------------------------------------------

// TestCurrentInstalledVersionFound verifies that the version is returned
// when the package is present in the status data.
func TestCurrentInstalledVersionFound(t *testing.T) {
	t.Parallel()

	status := "Package: hello\n" +
		"Architecture: amd64\n" +
		"Version: 2.10-2\n" +
		"Status: install ok installed\n" +
		"\n"

	got := aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "hello", "amd64")
	assert.Equal(t, "2.10-2", got)
}

// TestCurrentInstalledVersionNotFound verifies that an empty string is
// returned when the package is absent from the status data.
func TestCurrentInstalledVersionNotFound(t *testing.T) {
	t.Parallel()

	status := "Package: hello\n" +
		"Architecture: amd64\n" +
		"Version: 2.10-2\n" +
		"Status: install ok installed\n" +
		"\n"

	got := aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "nonexistent", "amd64")
	assert.Equal(t, "", got)
}

// TestCurrentInstalledVersionFallbackNoArch verifies that the fallback
// lookup (without arch qualifier) works when the entry has no Architecture.
func TestCurrentInstalledVersionFallbackNoArch(t *testing.T) {
	t.Parallel()

	// Entry has no Architecture field → keyed as "hello" (no arch suffix).
	status := "Package: hello\n" +
		"Version: 1.0\n" +
		"Status: install ok installed\n" +
		"\n"

	// Query with arch — should fall back to the bare "hello" key.
	got := aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "hello", "amd64")
	assert.Equal(t, "1.0", got)
}

// TestCurrentInstalledVersionEmptyStatus verifies that an empty status
// string returns an empty version.
func TestCurrentInstalledVersionEmptyStatus(t *testing.T) {
	t.Parallel()

	got := aptinstall.CurrentInstalledVersionFromStatusForTesting("", "hello", "amd64")
	assert.Equal(t, "", got)
}

// TestCurrentInstalledVersionMultiplePackages verifies that the correct
// version is returned when multiple packages are present.
func TestCurrentInstalledVersionMultiplePackages(t *testing.T) {
	t.Parallel()

	status := "Package: alpha\n" +
		"Architecture: amd64\n" +
		"Version: 1.0\n" +
		"Status: install ok installed\n" +
		"\n" +
		"Package: beta\n" +
		"Architecture: amd64\n" +
		"Version: 2.0\n" +
		"Status: install ok installed\n" +
		"\n" +
		"Package: gamma\n" +
		"Architecture: amd64\n" +
		"Version: 3.0\n" +
		"Status: install ok installed\n" +
		"\n"

	assert.Equal(t, "1.0", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "alpha", "amd64"))
	assert.Equal(t, "2.0", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "beta", "amd64"))
	assert.Equal(t, "3.0", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "gamma", "amd64"))
	assert.Equal(t, "", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "delta", "amd64"))
}

// TestCurrentInstalledVersionArchQualifier verifies that two packages with
// the same name but different architectures are distinguished correctly.
func TestCurrentInstalledVersionArchQualifier(t *testing.T) {
	t.Parallel()

	status := "Package: libc6\n" +
		"Architecture: amd64\n" +
		"Version: 2.31-13\n" +
		"Status: install ok installed\n" +
		"\n" +
		"Package: libc6\n" +
		"Architecture: arm64\n" +
		"Version: 2.31-14\n" +
		"Status: install ok installed\n" +
		"\n"

	assert.Equal(t, "2.31-13", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "libc6", "amd64"))
	assert.Equal(t, "2.31-14", aptinstall.CurrentInstalledVersionFromStatusForTesting(status, "libc6", "arm64"))
}

// ---------------------------------------------------------------------------
// runMaintainerScript
// ---------------------------------------------------------------------------

// TestRunMaintainerScriptNoScriptlet verifies that a missing scriptlet is a
// no-op (returns nil) — packages that ship no preinst/postinst must not fail.
func TestRunMaintainerScriptNoScriptlet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "preinst", "hello", "amd64",
		map[string]string{}, // no scriptlets
		"", "",
	)
	require.NoError(t, err)
}

// TestRunMaintainerScriptEmptyBody verifies that an empty scriptlet body is
// treated as absent (no-op).
func TestRunMaintainerScriptEmptyBody(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "postinst", "hello", "amd64",
		map[string]string{"postinst": ""},
		"", "",
	)
	require.NoError(t, err)
}

// TestRunMaintainerScriptUnknownPhase verifies that an unrecognised phase
// name returns an error.
func TestRunMaintainerScriptUnknownPhase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "badphase", "hello", "amd64",
		map[string]string{"badphase": "#!/bin/sh\nexit 0\n"},
		"", "",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown maintainer phase")
}

// TestRunMaintainerScriptPreinstFreshInstall verifies that a preinst
// scriptlet is invoked with action "install" on a fresh install (no old
// version).  The script writes its $1 argument to a temp file so we can
// assert the action without relying on /var/lib/dpkg/info.
func TestRunMaintainerScriptPreinstFreshInstall(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	actionFile := filepath.Join(dir, "action")

	// The scriptlet writes its first positional arg ($1 = action) to a file.
	// scriptletPathForPackage will look for the file at /var/lib/dpkg/info/hello.preinst
	// but runMaintainerScript calls runScriptlet which requires the file to
	// exist on disk.  We write it to the dpkg info dir via the scriptlet body
	// embedded in the debContents — but runMaintainerScript itself calls
	// scriptletPathForPackage to derive the path, so the script must be at
	// /var/lib/dpkg/info/<pkg>.preinst.
	//
	// To avoid touching /var/lib/dpkg on the test host we use a trick: we
	// write the script to a temp dir and then call RunScriptletForTesting
	// directly (which is already tested in scriptlet_status_test.go).
	// Here we test the *logic* of runMaintainerScript (action selection,
	// args) by observing the file the script writes.
	//
	// Because runMaintainerScript calls scriptletPathForPackage (which
	// hard-codes /var/lib/dpkg/info), we can only run this test when we
	// have write access to that directory (i.e. root inside a container).
	// On a developer workstation we skip gracefully.
	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); err != nil {
		t.Skipf("skipping: %s not accessible (%v)", infoDir, err)
	}

	scriptPath := filepath.Join(infoDir, "hello-test-extra.preinst")

	script := "#!/bin/sh\necho \"$1\" > " + actionFile + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Skipf("skipping: cannot write to %s (%v)", infoDir, err)
	}

	defer func() { _ = os.Remove(scriptPath) }()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "preinst", "hello-test-extra", "amd64",
		map[string]string{"preinst": script},
		"", // no old version → fresh install
		"",
	)
	require.NoError(t, err)

	data, err := os.ReadFile(actionFile)
	require.NoError(t, err)
	assert.Equal(t, "install", strings.TrimSpace(string(data)))
}

// TestRunMaintainerScriptPreinstUpgrade verifies that a preinst scriptlet
// is invoked with action "upgrade" and the old version when upgrading.
func TestRunMaintainerScriptPreinstUpgrade(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); err != nil {
		t.Skipf("skipping: %s not accessible (%v)", infoDir, err)
	}

	dir := t.TempDir()
	actionFile := filepath.Join(dir, "action")
	oldVerFile := filepath.Join(dir, "oldver")

	script := "#!/bin/sh\necho \"$1\" > " + actionFile + "\necho \"$2\" > " + oldVerFile + "\n"
	scriptPath := filepath.Join(infoDir, "hello-test-upgrade.preinst")

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Skipf("skipping: cannot write to %s (%v)", infoDir, err)
	}

	defer func() { _ = os.Remove(scriptPath) }()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "preinst", "hello-test-upgrade", "amd64",
		map[string]string{"preinst": script},
		"",
		"1.0", // old version present → upgrade
	)
	require.NoError(t, err)

	action, err := os.ReadFile(actionFile)
	require.NoError(t, err)
	assert.Equal(t, "upgrade", strings.TrimSpace(string(action)))

	oldVer, err := os.ReadFile(oldVerFile)
	require.NoError(t, err)
	assert.Equal(t, "1.0", strings.TrimSpace(string(oldVer)))
}

// TestRunMaintainerScriptPostinstConfigure verifies that a postinst
// scriptlet is invoked with action "configure" (no old version on fresh
// install).
func TestRunMaintainerScriptPostinstConfigure(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); err != nil {
		t.Skipf("skipping: %s not accessible (%v)", infoDir, err)
	}

	dir := t.TempDir()
	actionFile := filepath.Join(dir, "action")

	script := "#!/bin/sh\necho \"$1\" > " + actionFile + "\n"
	scriptPath := filepath.Join(infoDir, "hello-test-postinst.postinst")

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Skipf("skipping: cannot write to %s (%v)", infoDir, err)
	}

	defer func() { _ = os.Remove(scriptPath) }()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "postinst", "hello-test-postinst", "amd64",
		map[string]string{"postinst": script},
		"",
		"", // no old version
	)
	require.NoError(t, err)

	data, err := os.ReadFile(actionFile)
	require.NoError(t, err)
	assert.Equal(t, "configure", strings.TrimSpace(string(data)))
}

// TestRunMaintainerScriptPostinstConfigureWithOldVersion verifies that a
// postinst scriptlet receives the old version as $2 when upgrading.
func TestRunMaintainerScriptPostinstConfigureWithOldVersion(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); err != nil {
		t.Skipf("skipping: %s not accessible (%v)", infoDir, err)
	}

	dir := t.TempDir()
	actionFile := filepath.Join(dir, "action")
	oldVerFile := filepath.Join(dir, "oldver")

	script := "#!/bin/sh\necho \"$1\" > " + actionFile + "\necho \"$2\" > " + oldVerFile + "\n"
	scriptPath := filepath.Join(infoDir, "hello-test-postinst-upgrade.postinst")

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Skipf("skipping: cannot write to %s (%v)", infoDir, err)
	}

	defer func() { _ = os.Remove(scriptPath) }()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "postinst", "hello-test-postinst-upgrade", "amd64",
		map[string]string{"postinst": script},
		"",
		"1.0", // old version
	)
	require.NoError(t, err)

	action, err := os.ReadFile(actionFile)
	require.NoError(t, err)
	assert.Equal(t, "configure", strings.TrimSpace(string(action)))

	oldVer, err := os.ReadFile(oldVerFile)
	require.NoError(t, err)
	assert.Equal(t, "1.0", strings.TrimSpace(string(oldVer)))
}

// TestRunMaintainerScriptFailingScript verifies that a non-zero exit from
// the scriptlet propagates as an error.
func TestRunMaintainerScriptFailingScript(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); err != nil {
		t.Skipf("skipping: %s not accessible (%v)", infoDir, err)
	}

	script := "#!/bin/sh\nexit 42\n"
	scriptPath := filepath.Join(infoDir, "hello-test-fail.postinst")

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Skipf("skipping: cannot write to %s (%v)", infoDir, err)
	}

	defer func() { _ = os.Remove(scriptPath) }()

	ctx := context.Background()
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "postinst", "hello-test-fail", "amd64",
		map[string]string{"postinst": script},
		"", "",
	)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// RefreshLDCache
// ---------------------------------------------------------------------------

// TestRefreshLDCacheDoesNotPanic verifies that RefreshLDCache never panics,
// regardless of whether ldconfig is present on the test host.
func TestRefreshLDCacheDoesNotPanic(t *testing.T) {
	t.Parallel()

	// Should not panic even if ldconfig is absent or fails.
	assert.NotPanics(t, func() {
		aptinstall.RefreshLDCache()
	})
}

// ---------------------------------------------------------------------------
// InstallWithOptions — option validation
// ---------------------------------------------------------------------------

// TestInstallWithOptionsEmptyNames verifies that an empty package list is a
// no-op (returns nil immediately, before any lock or cache access).
func TestInstallWithOptionsEmptyNames(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := aptinstall.InstallWithOptions(ctx, []string{}, aptinstall.Options{
		RootDir:          t.TempDir(),
		AllowRootInstall: false,
		RunLDConfig:      false,
	})
	require.NoError(t, err)
}

// TestInstallWithOptionsRootRefused verifies that InstallWithOptions refuses
// to install into "/" without AllowRootInstall.
func TestInstallWithOptionsRootRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := aptinstall.InstallWithOptions(ctx, []string{"some-pkg"}, aptinstall.Options{
		RootDir:          "/",
		AllowRootInstall: false,
		RunLDConfig:      false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AllowRootInstall")
}
