// status_fs_test.go covers the filesystem-touching functions in status.go:
//   - readDpkgStatus (via ReadDpkgStatusFromPathForTesting)
//   - writeDpkgStatus (via WriteDpkgStatusToPathForTesting)
//   - writeAllStatusEntries — error path (write to a read-only file)
//   - writeDeb822Field — remaining uncovered branches
//   - updateDpkgStatusForPackage (via UpdateDpkgStatusForPackageAtPathForTesting)
//   - ensureDpkgDirs (via EnsureDpkgDirsForTesting — skipped when non-root)
//   - acquireDpkgLock / Release (via AcquireDpkgLockForTesting)
//   - writeDpkgInfoFiles (via WriteDpkgInfoFilesForTesting)
package aptinstall_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// ── readDpkgStatus ────────────────────────────────────────────────────────────

// TestReadDpkgStatusNonExistentFile verifies that a missing status file
// returns an empty map without an error (the "first install" case).
func TestReadDpkgStatusNonExistentFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status") // does not exist

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected empty map for missing file, got %d entries", len(entries))
	}
}

// TestReadDpkgStatusSingleEntry verifies that a single-stanza status file
// is parsed into exactly one entry with all fields intact.
func TestReadDpkgStatusSingleEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	content := "Package: hello\n" +
		"Version: 1.0\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries["hello:amd64"]
	if e == nil {
		t.Fatal("entry 'hello:amd64' not found")
	}

	if e["Package"] != "hello" {
		t.Errorf("Package: want 'hello', got %q", e["Package"])
	}

	if e["Version"] != "1.0" {
		t.Errorf("Version: want '1.0', got %q", e["Version"])
	}

	if e["Status"] != "install ok installed" {
		t.Errorf("Status: want 'install ok installed', got %q", e["Status"])
	}
}

// TestReadDpkgStatusMultipleEntries verifies that multiple stanzas are all
// parsed and keyed correctly.
func TestReadDpkgStatusMultipleEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	content := "Package: alpha\n" +
		"Version: 1.0\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"\n" +
		"Package: beta\n" +
		"Version: 2.0\n" +
		"Architecture: arm64\n" +
		"Status: install ok installed\n" +
		"\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries["alpha:amd64"] == nil {
		t.Error("alpha:amd64 entry missing")
	}

	if entries["beta:arm64"] == nil {
		t.Error("beta:arm64 entry missing")
	}
}

// TestReadDpkgStatusNoArchKey verifies that a stanza without an Architecture
// field is keyed by package name alone.
func TestReadDpkgStatusNoArchKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	content := "Package: noarch\n" +
		"Version: 3.0\n" +
		"Status: install ok installed\n" +
		"\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entries["noarch"] == nil {
		t.Error("entry keyed by package name alone not found")
	}
}

// TestReadDpkgStatusPreservesLastField is a regression test: the last field
// of a stanza (often Description) must not be silently dropped on parse.
func TestReadDpkgStatusPreservesLastField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Description is the last field — no trailing blank line after it.
	content := "Package: lastfield\n" +
		"Version: 1.0\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"Description: synopsis line\n" +
		" continuation line\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	e := entries["lastfield:amd64"]
	if e == nil {
		t.Fatal("entry not found")
	}

	if !strings.Contains(e["Description"], "synopsis line") {
		t.Errorf("Description synopsis lost; got %q", e["Description"])
	}

	if !strings.Contains(e["Description"], "continuation line") {
		t.Errorf("Description continuation lost; got %q", e["Description"])
	}
}

// TestReadDpkgStatusEmptyFile verifies that an empty status file returns an
// empty map without error.
func TestReadDpkgStatusEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected empty map for empty file, got %d entries", len(entries))
	}
}

// ── writeDpkgStatus (atomic write) ───────────────────────────────────────────

// TestWriteDpkgStatusToPathCreatesFile verifies that WriteDpkgStatusToPath
// creates the status file and writes entries correctly.
func TestWriteDpkgStatusToPathCreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	data := map[string]map[string]string{
		"hello:amd64": {
			"Package":      "hello",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("status file not created: %v", err)
	}

	// Verify no tmp file left behind.
	if _, err := os.Stat(path + ".dpkg-tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should have been renamed away")
	}
}

// TestWriteDpkgStatusToPathRoundTrip verifies that entries written with
// WriteDpkgStatusToPath can be read back intact.
func TestWriteDpkgStatusToPathRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	original := map[string]map[string]string{
		"curl:amd64": {
			"Package":      "curl",
			"Version":      "7.88.1",
			"Architecture": "amd64",
			"Status":       "install ok installed",
			"Depends":      "libc6, libssl3",
			"Description":  "command line tool for transferring data with URL syntax",
		},
		"wget:amd64": {
			"Package":      "wget",
			"Version":      "1.21.3",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, original); err != nil {
		t.Fatalf("write error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if len(readBack) != 2 {
		t.Fatalf("expected 2 entries after round-trip, got %d", len(readBack))
	}

	curl := readBack["curl:amd64"]
	if curl == nil {
		t.Fatal("curl:amd64 missing after round-trip")
	}

	if curl["Version"] != "7.88.1" {
		t.Errorf("curl Version: want '7.88.1', got %q", curl["Version"])
	}

	if curl["Depends"] != "libc6, libssl3" {
		t.Errorf("curl Depends: want 'libc6, libssl3', got %q", curl["Depends"])
	}

	if readBack["wget:amd64"] == nil {
		t.Error("wget:amd64 missing after round-trip")
	}
}

// TestWriteDpkgStatusToPathOverwritesExisting verifies that a second write
// replaces the previous content atomically.
func TestWriteDpkgStatusToPathOverwritesExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	first := map[string]map[string]string{
		"old:amd64": {
			"Package":      "old",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, first); err != nil {
		t.Fatalf("first write error: %v", err)
	}

	second := map[string]map[string]string{
		"new:amd64": {
			"Package":      "new",
			"Version":      "2.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, second); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if readBack["old:amd64"] != nil {
		t.Error("old entry should have been replaced")
	}

	if readBack["new:amd64"] == nil {
		t.Error("new entry should be present")
	}
}

// TestWriteDpkgStatusToPathEmptyMap verifies that writing an empty map
// produces an empty (but valid) status file.
func TestWriteDpkgStatusToPathEmptyMap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, map[string]map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("status file not created: %v", err)
	}

	if info.Size() != 0 {
		t.Errorf("expected empty file, got %d bytes", info.Size())
	}
}

// ── writeDeb822Field — remaining branches ─────────────────────────────────────

// TestWriteDeb822FieldMultilineWithEmptyParagraph verifies that an embedded
// empty line in a multi-line value is emitted as " .\n" (deb822 paragraph break).
func TestWriteDeb822FieldMultilineWithEmptyParagraph(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	// Value with an empty intermediate line (paragraph break).
	value := "first line\n\nsecond paragraph"

	if err := aptinstall.WriteDeb822FieldForTesting(f, "Description", value); err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := "Description: first line\n .\n second paragraph\n"
	if string(got) != want {
		t.Fatalf("paragraph break wrong:\n--- want ---\n%q\n--- got ---\n%q", want, string(got))
	}
}

// TestWriteDeb822FieldMultilineNoEmptyLines verifies that a multi-line value
// without empty lines gets a leading space on every continuation line.
func TestWriteDeb822FieldMultilineNoEmptyLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	value := "line one\nline two\nline three"

	if err := aptinstall.WriteDeb822FieldForTesting(f, "Description", value); err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := "Description: line one\n line two\n line three\n"
	if string(got) != want {
		t.Fatalf("multi-line no-empty wrong:\n--- want ---\n%q\n--- got ---\n%q", want, string(got))
	}
}

// ── updateDpkgStatusForPackage ────────────────────────────────────────────────

// TestUpdateDpkgStatusForPackageInsertsNew verifies that a new package is
// inserted into an existing status file.
func TestUpdateDpkgStatusForPackageInsertsNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Seed with one existing entry.
	seed := map[string]map[string]string{
		"existing:amd64": {
			"Package":      "existing",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, seed); err != nil {
		t.Fatal(err)
	}

	control := "Package: newpkg\nVersion: 2.0\nArchitecture: amd64\n"

	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		path, "newpkg", "amd64", control, "install ok installed",
	); err != nil {
		t.Fatalf("update error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if len(readBack) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(readBack))
	}

	newEntry := readBack["newpkg:amd64"]
	if newEntry == nil {
		t.Fatal("newpkg:amd64 not found after update")
	}

	if newEntry["Status"] != "install ok installed" {
		t.Errorf("Status: want 'install ok installed', got %q", newEntry["Status"])
	}

	if newEntry["Version"] != "2.0" {
		t.Errorf("Version: want '2.0', got %q", newEntry["Version"])
	}
}

// TestUpdateDpkgStatusForPackageUpdatesExisting verifies that an existing
// entry is replaced (not duplicated) when the same package is updated.
func TestUpdateDpkgStatusForPackageUpdatesExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	seed := map[string]map[string]string{
		"mypkg:amd64": {
			"Package":      "mypkg",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(path, seed); err != nil {
		t.Fatal(err)
	}

	control := "Package: mypkg\nVersion: 2.0\nArchitecture: amd64\n"

	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		path, "mypkg", "amd64", control, "install ok installed",
	); err != nil {
		t.Fatalf("update error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if len(readBack) != 1 {
		t.Fatalf("expected 1 entry (no duplicate), got %d", len(readBack))
	}

	if readBack["mypkg:amd64"]["Version"] != "2.0" {
		t.Errorf("Version should be updated to '2.0', got %q", readBack["mypkg:amd64"]["Version"])
	}
}

// TestUpdateDpkgStatusForPackageNoArch verifies that a package without an
// architecture is keyed by name alone.
func TestUpdateDpkgStatusForPackageNoArch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	control := "Package: noarchpkg\nVersion: 1.0\n"

	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		path, "noarchpkg", "", control, "install ok installed",
	); err != nil {
		t.Fatalf("update error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if readBack["noarchpkg"] == nil {
		t.Error("noarchpkg entry not found (expected key without arch suffix)")
	}
}

// TestUpdateDpkgStatusForPackageFromEmptyFile verifies that the function
// works correctly when the status file does not yet exist.
func TestUpdateDpkgStatusForPackageFromEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status") // does not exist

	control := "Package: firstpkg\nVersion: 1.0\nArchitecture: amd64\n"

	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		path, "firstpkg", "amd64", control, "install ok installed",
	); err != nil {
		t.Fatalf("update error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if readBack["firstpkg:amd64"] == nil {
		t.Error("firstpkg:amd64 not found")
	}
}

// TestUpdateDpkgStatusForPackageStatusFieldOverrides verifies that the
// Status field from the control file is overridden by the explicit status arg.
func TestUpdateDpkgStatusForPackageStatusFieldOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Control file has a different Status value — it should be overridden.
	control := "Package: mypkg\nVersion: 1.0\nArchitecture: amd64\nStatus: deinstall ok config-files\n"

	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		path, "mypkg", "amd64", control, "install ok installed",
	); err != nil {
		t.Fatalf("update error: %v", err)
	}

	readBack, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	e := readBack["mypkg:amd64"]
	if e == nil {
		t.Fatal("entry not found")
	}

	if e["Status"] != "install ok installed" {
		t.Errorf("Status should be overridden to 'install ok installed', got %q", e["Status"])
	}
}

// ── acquireDpkgLock / Release ─────────────────────────────────────────────────

// TestAcquireDpkgLockAndRelease verifies that acquireDpkgLock returns a
// non-nil handle and that Release does not panic.
// When running as non-root (no write access to /var/lib/dpkg/lock), the
// function returns a best-effort no-op lock — that is also valid.
func TestAcquireDpkgLockAndRelease(t *testing.T) {
	t.Parallel()

	lock, err := aptinstall.AcquireDpkgLockForTesting()
	if err != nil {
		t.Fatalf("acquireDpkgLock returned error: %v", err)
	}

	// Release must not panic regardless of whether we got a real or no-op lock.
	lock.Release()
}

// TestAcquireDpkgLockReleaseIsIdempotent verifies that calling Release twice
// does not panic (nil-guard in the implementation).
func TestAcquireDpkgLockReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	lock, err := aptinstall.AcquireDpkgLockForTesting()
	if err != nil {
		t.Fatalf("acquireDpkgLock returned error: %v", err)
	}

	lock.Release()
	lock.Release() // second call must not panic
}

// ── writeDpkgInfoFiles ────────────────────────────────────────────────────────

// TestWriteDpkgInfoFilesCreatesListFile verifies that the .list file is
// written with the correct file paths.
// This test requires write access to /var/lib/dpkg/info; it is skipped
// when running as a non-root user outside a build container.
func TestWriteDpkgInfoFilesCreatesListFile(t *testing.T) {
	t.Parallel()

	// Check write access to /var/lib/dpkg/info.
	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); os.IsNotExist(err) {
		t.Skip("skipping: /var/lib/dpkg/info does not exist (not a Debian host)")
	}

	testFile := filepath.Join(infoDir, ".yap-write-test")

	f, err := os.Create(testFile)
	if err != nil {
		t.Skipf("skipping: no write access to %s: %v", infoDir, err)
	}

	_ = f.Close()
	_ = os.Remove(testFile)

	pkgName := "yap-test-infopkg"
	arch := "amd64"

	contents := &aptinstall.DebContentsForTesting{
		Control: "Package: " + pkgName + "\nVersion: 1.0\nArchitecture: " + arch + "\n",
		Files: []string{
			"/usr/bin/yap-test-infopkg",
			"/usr/share/doc/yap-test-infopkg/README",
		},
		Md5sums: "abc123  usr/bin/yap-test-infopkg\n" +
			"def456  usr/share/doc/yap-test-infopkg/README\n",
	}

	if err := aptinstall.WriteDpkgInfoFilesForTesting(pkgName, arch, contents); err != nil {
		t.Fatalf("WriteDpkgInfoFiles error: %v", err)
	}

	// Verify .list file.
	listPath := filepath.Join(infoDir, pkgName+".list")

	t.Cleanup(func() {
		_ = os.Remove(listPath)
		_ = os.Remove(filepath.Join(infoDir, pkgName+".md5sums"))
	})

	listData, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("could not read .list file: %v", err)
	}

	listContent := string(listData)

	if !strings.Contains(listContent, "/usr/bin/yap-test-infopkg") {
		t.Error(".list file missing /usr/bin/yap-test-infopkg")
	}

	if !strings.Contains(listContent, "/usr/share/doc/yap-test-infopkg/README") {
		t.Error(".list file missing /usr/share/doc/yap-test-infopkg/README")
	}

	// Verify .md5sums file.
	md5Path := filepath.Join(infoDir, pkgName+".md5sums")

	md5Data, err := os.ReadFile(md5Path)
	if err != nil {
		t.Fatalf("could not read .md5sums file: %v", err)
	}

	if !strings.Contains(string(md5Data), "abc123") {
		t.Error(".md5sums file missing expected hash")
	}
}

// TestWriteDpkgInfoFilesWithScriptlets verifies that scriptlet files are
// written with the correct names and content.
// Skipped when /var/lib/dpkg/info is not writable.
func TestWriteDpkgInfoFilesWithScriptlets(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); os.IsNotExist(err) {
		t.Skip("skipping: /var/lib/dpkg/info does not exist")
	}

	testFile := filepath.Join(infoDir, ".yap-write-test2")

	f, err := os.Create(testFile)
	if err != nil {
		t.Skipf("skipping: no write access to %s: %v", infoDir, err)
	}

	_ = f.Close()
	_ = os.Remove(testFile)

	pkgName := "yap-test-scriptpkg"
	arch := "amd64"

	contents := &aptinstall.DebContentsForTesting{
		Control: "Package: " + pkgName + "\nVersion: 1.0\nArchitecture: " + arch + "\n",
		Files:   []string{"/usr/bin/yap-test-scriptpkg"},
		Scriptlets: map[string]string{
			"postinst": "#!/bin/sh\necho 'postinst called'\n",
			"prerm":    "#!/bin/sh\necho 'prerm called'\n",
		},
	}

	if err := aptinstall.WriteDpkgInfoFilesForTesting(pkgName, arch, contents); err != nil {
		t.Fatalf("WriteDpkgInfoFiles error: %v", err)
	}

	listPath := filepath.Join(infoDir, pkgName+".list")
	postinstPath := filepath.Join(infoDir, pkgName+".postinst")
	prermPath := filepath.Join(infoDir, pkgName+".prerm")

	t.Cleanup(func() {
		_ = os.Remove(listPath)
		_ = os.Remove(postinstPath)
		_ = os.Remove(prermPath)
	})

	// Verify postinst.
	postinstData, err := os.ReadFile(postinstPath)
	if err != nil {
		t.Fatalf("could not read .postinst: %v", err)
	}

	if !strings.Contains(string(postinstData), "postinst called") {
		t.Error(".postinst content wrong")
	}

	// Verify prerm.
	prermData, err := os.ReadFile(prermPath)
	if err != nil {
		t.Fatalf("could not read .prerm: %v", err)
	}

	if !strings.Contains(string(prermData), "prerm called") {
		t.Error(".prerm content wrong")
	}
}

// TestWriteDpkgInfoFilesWithTriggers verifies that the .triggers file is
// written when Triggers content is present.
// Skipped when /var/lib/dpkg/info is not writable.
func TestWriteDpkgInfoFilesWithTriggers(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); os.IsNotExist(err) {
		t.Skip("skipping: /var/lib/dpkg/info does not exist")
	}

	testFile := filepath.Join(infoDir, ".yap-write-test3")

	f, err := os.Create(testFile)
	if err != nil {
		t.Skipf("skipping: no write access to %s: %v", infoDir, err)
	}

	_ = f.Close()
	_ = os.Remove(testFile)

	pkgName := "yap-test-triggerpkg"
	arch := "amd64"

	contents := &aptinstall.DebContentsForTesting{
		Control:  "Package: " + pkgName + "\nVersion: 1.0\nArchitecture: " + arch + "\n",
		Files:    []string{"/usr/bin/yap-test-triggerpkg"},
		Triggers: "interest /usr/share/fonts\n",
	}

	if err := aptinstall.WriteDpkgInfoFilesForTesting(pkgName, arch, contents); err != nil {
		t.Fatalf("WriteDpkgInfoFiles error: %v", err)
	}

	listPath := filepath.Join(infoDir, pkgName+".list")
	triggersPath := filepath.Join(infoDir, pkgName+".triggers")

	t.Cleanup(func() {
		_ = os.Remove(listPath)
		_ = os.Remove(triggersPath)
	})

	triggersData, err := os.ReadFile(triggersPath)
	if err != nil {
		t.Fatalf("could not read .triggers: %v", err)
	}

	if !strings.Contains(string(triggersData), "interest /usr/share/fonts") {
		t.Error(".triggers content wrong")
	}
}

// TestWriteDpkgInfoFilesWithConffiles verifies that the .conffiles file is
// written when Conffiles content is present.
// Skipped when /var/lib/dpkg/info is not writable.
func TestWriteDpkgInfoFilesWithConffiles(t *testing.T) {
	t.Parallel()

	infoDir := "/var/lib/dpkg/info"
	if _, err := os.Stat(infoDir); os.IsNotExist(err) {
		t.Skip("skipping: /var/lib/dpkg/info does not exist")
	}

	testFile := filepath.Join(infoDir, ".yap-write-test4")

	f, err := os.Create(testFile)
	if err != nil {
		t.Skipf("skipping: no write access to %s: %v", infoDir, err)
	}

	_ = f.Close()
	_ = os.Remove(testFile)

	pkgName := "yap-test-confpkg"
	arch := "amd64"

	contents := &aptinstall.DebContentsForTesting{
		Control:   "Package: " + pkgName + "\nVersion: 1.0\nArchitecture: " + arch + "\n",
		Files:     []string{"/etc/yap-test-confpkg.conf"},
		Conffiles: "/etc/yap-test-confpkg.conf abc123\n",
	}

	if err := aptinstall.WriteDpkgInfoFilesForTesting(pkgName, arch, contents); err != nil {
		t.Fatalf("WriteDpkgInfoFiles error: %v", err)
	}

	listPath := filepath.Join(infoDir, pkgName+".list")
	confPath := filepath.Join(infoDir, pkgName+".conffiles")

	t.Cleanup(func() {
		_ = os.Remove(listPath)
		_ = os.Remove(confPath)
	})

	confData, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("could not read .conffiles: %v", err)
	}

	if !strings.Contains(string(confData), "/etc/yap-test-confpkg.conf") {
		t.Error(".conffiles content wrong")
	}
}

// ── handleDpkgStatusLine — remaining branch ───────────────────────────────────

// TestHandleDpkgStatusLineNoColonLine verifies that a line without a colon
// (and not a continuation) is handled gracefully — the field is cleared.
func TestHandleDpkgStatusLineNoColonLine(t *testing.T) {
	t.Parallel()

	// A line with no colon and no leading space is a malformed field.
	// The parser should not crash and should produce no entry for it.
	content := "Package: malformed\n" +
		"Version: 1.0\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"NOCOLON\n" + // malformed line — no colon
		"\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The entry should still be parsed (the malformed line is ignored).
	e := entries["malformed:amd64"]
	if e == nil {
		t.Fatal("entry not found despite malformed line")
	}

	if e["Package"] != "malformed" {
		t.Errorf("Package: want 'malformed', got %q", e["Package"])
	}
}

// ── writeAllStatusEntries — error path ───────────────────────────────────────

// TestWriteAllStatusEntriesWriteError verifies that a write error is
// propagated correctly (write to a read-only file descriptor).
func TestWriteAllStatusEntriesWriteError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "readonly")

	// Create the file, then open it read-only.
	if err := os.WriteFile(path, []byte(""), 0o444); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_RDONLY, 0o444) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	data := map[string]map[string]string{
		"pkg": {"Package": "pkg", "Version": "1.0"},
	}

	err = aptinstall.WriteAllStatusEntriesForTesting(f, data)
	if err == nil {
		t.Error("expected error writing to read-only file, got nil")
	}
}
