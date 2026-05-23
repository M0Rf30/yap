// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptinstall

import (
	"archive/tar"
	"context"
	"io"
	"maps"
	"os"
)

// WriteDeb822FieldForTesting exposes the deb822 field emitter so round-
// trip regression tests can assert that multi-line values come out with
// the leading-space continuation marker dpkg requires.
func WriteDeb822FieldForTesting(f *os.File, field, value string) error {
	return writeDeb822Field(f, field, value)
}

// ParseControlForTesting exposes parseControl for unit tests.
func ParseControlForTesting(control string) map[string]string {
	return parseControl(control)
}

// ParseDEBForTesting exposes parseDEB for unit tests.
func ParseDEBForTesting(debPath string) (*debContents, error) {
	return parseDEB(debPath)
}

// DecompressStreamForTesting exposes decompressStream for unit tests.
func DecompressStreamForTesting(r io.Reader, name string) (io.ReadCloser, error) {
	return decompressStream(r, name)
}

// ExtractDataTarWithConffilesForTesting exposes the conffile-aware
// extractor so safety tests can assert that conffiles are NOT overwritten
// (C-6 regression).
func ExtractDataTarWithConffilesForTesting(dataTarPath, destDir string, conffiles []string) error {
	return extractDataTarWithConffiles(dataTarPath, destDir, conffiles)
}

// SafeJoinForTesting exposes safeJoin for path-traversal regression tests.
func SafeJoinForTesting(destDir, entry string) (string, error) {
	return safeJoin(destDir, entry)
}

// SafeSymlinkTargetForTesting exposes safeSymlinkTarget for tests.
func SafeSymlinkTargetForTesting(destDir, linkPath, target string) error {
	return safeSymlinkTarget(destDir, linkPath, target)
}

// ExtractDataTarForTesting exposes extractDataTar for unit tests.
func ExtractDataTarForTesting(debPath, destDir string, conffiles []string) error {
	return extractDataTar(debPath, destDir, conffiles)
}

// ExtractTarDirForTesting exposes extractTarDir for unit tests.
func ExtractTarDirForTesting(hdr *tar.Header, fullPath string) error {
	return extractTarDir(hdr, fullPath, make(map[string]bool))
}

// ExtractTarSymlinkForTesting exposes extractTarSymlink for unit tests.
func ExtractTarSymlinkForTesting(hdr *tar.Header, destDir, fullPath string) error {
	return extractTarSymlink(hdr, destDir, fullPath, make(map[string]bool))
}

// ReadDpkgStatusFromStringForTesting parses a synthetic /var/lib/dpkg/status
// payload (avoiding the real filesystem) so tests can assert the parser
// preserves every field of every stanza across a round-trip.
func ReadDpkgStatusFromStringForTesting(data string) (map[string]map[string]string, error) {
	entries := make(map[string]*dpkgStatusEntry)
	st := dpkgParseState{}

	for _, line := range splitLines(data) {
		handleDpkgStatusLine(line, &st, entries)
	}

	flushDpkgStatusEntry(&st, entries)

	out := make(map[string]map[string]string, len(entries))
	for k, v := range entries {
		out[k] = v.fields
	}

	return out, nil
}

func splitLines(s string) []string {
	var lines []string

	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

// HasEnvKeyForTesting exposes hasEnvKey for unit tests.
func HasEnvKeyForTesting(env []string, key string) bool {
	return hasEnvKey(env, key)
}

// ResolveRootDirForTesting exposes resolveRootDir for unit tests.
func ResolveRootDirForTesting(rootDir string, allowRoot bool) (string, error) {
	return resolveRootDir(Options{RootDir: rootDir, AllowRootInstall: allowRoot})
}

// CurrentInstalledVersionFromStatusForTesting reads the installed version
// for a package from a synthetic dpkg status string (avoids touching the
// real /var/lib/dpkg/status on the test host).
func CurrentInstalledVersionFromStatusForTesting(statusData, pkgName, arch string) string {
	entries := make(map[string]*dpkgStatusEntry)
	st := dpkgParseState{}

	for _, line := range splitLines(statusData) {
		handleDpkgStatusLine(line, &st, entries)
	}

	flushDpkgStatusEntry(&st, entries)

	key := pkgName
	if arch != "" {
		key = pkgName + ":" + arch
	}

	if e, ok := entries[key]; ok {
		return e.fields["Version"]
	}

	if e, ok := entries[pkgName]; ok {
		return e.fields["Version"]
	}

	return ""
}

// RunMaintainerScriptForPhaseForTesting exposes runMaintainerScript for unit tests.
// It accepts a debContents-like struct so callers can inject scriptlet bodies.
func RunMaintainerScriptForPhaseForTesting(
	ctx context.Context,
	phase, pkgName, arch string,
	scriptlets map[string]string,
	control, oldVersion string,
) error {
	contents := &debContents{
		Scriptlets: scriptlets,
		Control:    control,
	}

	return runMaintainerScript(ctx, phase, pkgName, arch, contents, oldVersion)
}

// FilterScriptletEnvForTesting exposes filterScriptletEnv for unit tests.
func FilterScriptletEnvForTesting() []string {
	return filterScriptletEnv()
}

// ScriptletPathForPackageForTesting exposes scriptletPathForPackage for unit tests.
func ScriptletPathForPackageForTesting(pkgName, arch, control, scriptName string) string {
	return scriptletPathForPackage(pkgName, arch, control, scriptName)
}

// RunScriptletForTesting exposes runScriptlet for unit tests.
func RunScriptletForTesting(ctx context.Context, scriptPath, scriptName, pkgName, action string, args ...string) error {
	return runScriptlet(ctx, scriptPath, scriptName, pkgName, action, args...)
}

// WriteStatusEntryForTesting exposes writeStatusEntry for unit tests.
// It writes a single dpkg status entry with fields in canonical order.
func WriteStatusEntryForTesting(f *os.File, fields map[string]string) error {
	entry := &dpkgStatusEntry{fields: fields}

	return writeStatusEntry(f, entry)
}

// WriteAllStatusEntriesForTesting exposes writeAllStatusEntries for unit tests.
// It writes multiple entries in sorted key order, separated by blank lines.
func WriteAllStatusEntriesForTesting(f *os.File, data map[string]map[string]string) error {
	entries := make(map[string]*dpkgStatusEntry, len(data))
	for k, v := range data {
		entries[k] = &dpkgStatusEntry{fields: v}
	}

	return writeAllStatusEntries(f, entries)
}

// ReadDpkgStatusFromPathForTesting reads and parses a dpkg status file at
// the given path (instead of the hardcoded /var/lib/dpkg/status).
func ReadDpkgStatusFromPathForTesting(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // test helper
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]map[string]string{}, nil
		}

		return nil, err
	}

	entries := make(map[string]*dpkgStatusEntry)
	st := dpkgParseState{}

	for _, line := range splitLines(string(data)) {
		handleDpkgStatusLine(line, &st, entries)
	}

	flushDpkgStatusEntry(&st, entries)

	out := make(map[string]map[string]string, len(entries))
	for k, v := range entries {
		out[k] = v.fields
	}

	return out, nil
}

// WriteDpkgStatusToPathForTesting writes dpkg status entries to a custom
// path (instead of the hardcoded /var/lib/dpkg/status) for unit tests.
func WriteDpkgStatusToPathForTesting(statusPath string, data map[string]map[string]string) error {
	tmpPath := statusPath + ".dpkg-tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // test helper
	if err != nil {
		return err
	}

	entries := make(map[string]*dpkgStatusEntry, len(data))
	for k, v := range data {
		entries[k] = &dpkgStatusEntry{fields: v}
	}

	if err := writeAllStatusEntries(f, entries); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	return os.Rename(tmpPath, statusPath)
}

// UpdateDpkgStatusForPackageAtPathForTesting calls updateDpkgStatusForPackage
// but reads/writes to a custom status path instead of /var/lib/dpkg/status.
// It does this by writing the initial entries to the path, then calling the
// internal helper directly via the exposed write/read helpers.
func UpdateDpkgStatusForPackageAtPathForTesting(
	statusPath, pkgName, arch, control, status string,
) error {
	// Read existing entries from the custom path.
	data, err := ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		return err
	}

	// Parse the control file.
	controlFields := parseControl(control)

	// Build the new entry.
	entry := make(map[string]string, len(controlFields)+2)
	maps.Copy(entry, controlFields)

	entry["Status"] = status
	entry["Package"] = pkgName

	if arch != "" {
		entry["Architecture"] = arch
	}

	// Determine the key.
	key := pkgName
	if arch != "" {
		key = pkgName + ":" + arch
	}

	data[key] = entry

	return WriteDpkgStatusToPathForTesting(statusPath, data)
}

// EnsureDpkgDirsForTesting exposes ensureDpkgDirs for unit tests.
func EnsureDpkgDirsForTesting() error {
	return ensureDpkgDirs()
}

// AcquireDpkgLockForTesting exposes acquireDpkgLock for unit tests.
// Returns an opaque handle; call ReleaseDpkgLockForTesting to release it.
func AcquireDpkgLockForTesting() (interface{ Release() }, error) {
	return acquireDpkgLock()
}

// WriteDpkgInfoFilesForTesting exposes writeDpkgInfoFiles for unit tests.
func WriteDpkgInfoFilesForTesting(pkgName, arch string, contents *DebContentsForTesting) error {
	dc := &debContents{
		Control:    contents.Control,
		Md5sums:    contents.Md5sums,
		Conffiles:  contents.Conffiles,
		Scriptlets: contents.Scriptlets,
		Triggers:   contents.Triggers,
		Files:      contents.Files,
	}

	return writeDpkgInfoFiles(pkgName, arch, dc)
}

// DebContentsForTesting is an exported mirror of debContents for test use.
type DebContentsForTesting struct {
	Control    string
	Md5sums    string
	Conffiles  string
	Scriptlets map[string]string
	Triggers   string
	Files      []string
}
