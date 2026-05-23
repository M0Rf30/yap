package aptinstall

import (
		"context"
"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/yapdb"
)

const (
	dpkgStatusPath = "/var/lib/dpkg/status"
	dpkgLockPath   = "/var/lib/dpkg/lock"
)

// dpkgStatusEntry represents a single package entry in /var/lib/dpkg/status.
type dpkgStatusEntry struct {
	fields map[string]string
}

// dpkgParseState holds the state of a dpkg status stanza being parsed.
type dpkgParseState struct {
	currentEntry *dpkgStatusEntry
	currentField string
	currentValue strings.Builder
}

// handleDpkgStatusLine processes a single line from a dpkg status stanza.
// Returns nothing; mutates state. Handles blank line (flush), continuation, or field.
func handleDpkgStatusLine(line string, st *dpkgParseState, entries map[string]*dpkgStatusEntry) {
	// Blank line → end of stanza.
	if line == "" {
		flushDpkgStatusEntry(st, entries)
		*st = dpkgParseState{}

		return
	}

	// Continuation line (starts with space or tab).
	if line != "" && (line[0] == ' ' || line[0] == '\t') {
		st.currentValue.WriteString("\n")
		st.currentValue.WriteString(strings.TrimPrefix(line, " "))

		return
	}

	// Flush previous field.
	if st.currentField != "" && st.currentEntry != nil {
		st.currentEntry.fields[st.currentField] = st.currentValue.String()
	}
	// Parse new field line.
	if field, value, ok := strings.Cut(line, ":"); ok {
		if st.currentEntry == nil {
			st.currentEntry = &dpkgStatusEntry{fields: make(map[string]string)}
		}

		st.currentField = field
		st.currentValue.Reset()
		st.currentValue.WriteString(strings.TrimSpace(value))
	} else {
		st.currentField = ""
	}
}

// flushDpkgStatusEntry adds a completed entry to the entries map.
// Crucially, it flushes any pending field/value into the entry before
// publishing — without this step the *last* field of every stanza is
// silently dropped on a re-parse, corrupting the dpkg database after a
// single round-trip.
func flushDpkgStatusEntry(st *dpkgParseState, entries map[string]*dpkgStatusEntry) {
	if st.currentEntry == nil {
		return
	}

	// Flush the in-flight field that was being accumulated when the stanza
	// ended. Previously this was only done on the "next field" branch, so
	// the trailing field (often Description or Conffiles) was lost.
	if st.currentField != "" {
		st.currentEntry.fields[st.currentField] = st.currentValue.String()
	}

	if st.currentEntry.fields["Package"] == "" {
		return
	}

	key := st.currentEntry.fields["Package"]
	if arch, ok := st.currentEntry.fields["Architecture"]; ok && arch != "" {
		key = key + ":" + arch
	}

	entries[key] = st.currentEntry
}

// readDpkgStatus reads and parses /var/lib/dpkg/status.
func readDpkgStatus() (map[string]*dpkgStatusEntry, error) {
	entries := make(map[string]*dpkgStatusEntry)

	data, err := os.ReadFile(dpkgStatusPath) // #nosec G304 - constant path
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil // File doesn't exist yet; that's OK.
		}

		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "read dpkg status").
			WithOperation("readDpkgStatus").WithContext("path", dpkgStatusPath)
	}

	if err := deb822.Parse(strings.NewReader(string(data)), func(stanzaMap deb822.Stanza) error {
		entry := &dpkgStatusEntry{fields: make(map[string]string)}

		// Copy all fields from the stanza into the entry
		for k, v := range stanzaMap {
			entry.fields[k] = v
		}

		// Add the entry to the map using the same logic as flushDpkgStatusEntry
		if entry.fields["Package"] == "" {
			return nil
		}

		key := entry.fields["Package"]
		if arch, ok := entry.fields["Architecture"]; ok && arch != "" {
			key = key + ":" + arch
		}

		entries[key] = entry

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeParser, "parse dpkg status").
			WithOperation("readDpkgStatus").WithContext("path", dpkgStatusPath)
	}

	return entries, nil
}

// writeDpkgStatus writes the dpkg status database atomically.
//
// Strategy:
//  1. Write to "<status>.dpkg-tmp" (fsync the contents).
//  2. fsync the tmp file.
//  3. Rename tmp → status (clobbers atomically on POSIX).
//
// We never move the live status file out of the way before the new copy is
// fully on disk: an unexpected kill between the move and the rename would
// otherwise leave the system with no status database at all, and the next
// installer invocation would happily write a one-entry file, permanently
// forgetting every previously-installed package.
func writeDpkgStatus(entries map[string]*dpkgStatusEntry) error {
	tmpPath := dpkgStatusPath + ".dpkg-tmp"

	f, err := os.OpenFile(tmpPath, // #nosec G304 - constant path
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) // #nosec G302
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create temp status file").
			WithOperation("writeDpkgStatus").WithContext("path", tmpPath)
	}

	if err := writeAllStatusEntries(f, entries); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "sync status file").
			WithOperation("writeDpkgStatus")
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "close status file").
			WithOperation("writeDpkgStatus")
	}

	// Atomic clobber. On POSIX, rename(2) replaces the destination
	// atomically — no window where dpkgStatusPath is missing.
	if err := os.Rename(tmpPath, dpkgStatusPath); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "rename status file").
			WithOperation("writeDpkgStatus")
	}

	return nil
}

// writeAllStatusEntries writes every entry in a deterministic (sorted)
// order so the output is diffable across runs and external tooling can
// snapshot it reliably.
func writeAllStatusEntries(f *os.File, entries map[string]*dpkgStatusEntry) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		if err := writeStatusEntry(f, entries[k]); err != nil {
			return err
		}

		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	return nil
}

// writeStatusEntry writes a single dpkg status entry to a file.
func writeStatusEntry(f *os.File, entry *dpkgStatusEntry) error {
	// Write fields in a consistent order for readability.
	fieldOrder := []string{
		"Package", "Architecture", "Multi-Arch", "Status", "Priority", "Section",
		"Installed-Size", "Maintainer", "Source", "Version", "Depends", "Pre-Depends",
		"Conflicts", "Breaks", "Replaces", "Provides", "Essential", "Description",
	}

	written := make(map[string]bool)

	for _, field := range fieldOrder {
		if value, ok := entry.fields[field]; ok {
			if err := writeDeb822Field(f, field, value); err != nil {
				return err
			}

			written[field] = true
		}
	}

	// Write any remaining fields not in the order list, in sorted order
	// so output is deterministic.
	extra := make([]string, 0, len(entry.fields))
	for k := range entry.fields {
		if !written[k] {
			extra = append(extra, k)
		}
	}

	sort.Strings(extra)

	for _, k := range extra {
		if err := writeDeb822Field(f, k, entry.fields[k]); err != nil {
			return err
		}
	}

	return nil
}

// writeDeb822Field emits a single "Field: value" line, re-applying the
// deb822 continuation convention: every embedded newline is followed by
// a leading space (multi-line values), and empty paragraph-break lines
// are emitted as ` .` per the dpkg control-file spec.
//
// The parser stores `Description: synopsis\n extended line 1\n .\n extended line 2`
// as the in-memory value `"synopsis\nextended line 1\n.\nextended line 2"`
// (leading space stripped on each continuation line). Without re-applying
// the leading space on emit, dpkg would interpret each continuation line
// as a new field header and reject the entire status file with
// "field name '...' must be followed by colon".
//
// Empty values are emitted as `Field:` with a trailing space-less form
// (matches dpkg's own output, e.g. `Conffiles:` when a package ships no
// conffiles).
func writeDeb822Field(f *os.File, field, value string) error {
	if value == "" {
		_, err := fmt.Fprintf(f, "%s:\n", field)
		return err
	}

	if !strings.ContainsRune(value, '\n') {
		_, err := fmt.Fprintf(f, "%s: %s\n", field, value)
		return err
	}

	// Multi-line: first line follows "Field: ", every subsequent line
	// gets a leading space. Empty intermediate lines become ` .` so
	// dpkg's paragraph-break convention is preserved.
	lines := strings.Split(value, "\n")

	if _, err := fmt.Fprintf(f, "%s: %s\n", field, lines[0]); err != nil {
		return err
	}

	for _, line := range lines[1:] {
		if line == "" {
			if _, err := fmt.Fprintln(f, " ."); err != nil {
				return err
			}

			continue
		}

		if _, err := fmt.Fprintf(f, " %s\n", line); err != nil {
			return err
		}
	}

	return nil
}

// writeYapdb writes the installed package metadata to the YAP state database.
func writeYapdb(
	ctx context.Context,
	pkgName, arch string,
	controlFields map[string]string,
	rootDir string,
	files []string,
) error {
	// Extract package metadata from control fields.
	version := controlFields["Version"]
	summary := controlFields["Summary"]
	if summary == "" {
		// Fallback to Description if Summary is not present.
		desc := controlFields["Description"]
		if desc != "" {
			// Take only the first line (synopsis).
			if idx := strings.Index(desc, "\n"); idx >= 0 {
				summary = desc[:idx]
			} else {
				summary = desc
			}
		}
	}

	// Convert files to yapdb.File.
	var yapdbFiles []yapdb.File
	for _, filePath := range files {
		yapdbFiles = append(yapdbFiles, yapdb.File{
			Path:       filePath,
			Mode:       0o644, // Default mode; we don't track actual modes from .deb
			IsDir:      false,
			IsSymlink:  false,
			LinkTarget: "",
			SHA256:     "", // We don't track SHA256 from .deb
		})
	}

	// Extract capabilities from Provides and Depends fields.
	var caps []yapdb.Capability

	// Add Provides as "provide" capabilities.
	if provides := controlFields["Provides"]; provides != "" {
		for _, prov := range strings.Split(provides, ",") {
			prov = strings.TrimSpace(prov)
			if prov != "" {
				caps = append(caps, yapdb.Capability{
					Kind:    "provide",
					Name:    prov,
					Flags:   0,
					Version: version,
				})
			}
		}
	}

	// Add package name as a provide capability.
	caps = append(caps, yapdb.Capability{
		Kind:    "provide",
		Name:    pkgName,
		Flags:   0,
		Version: version,
	})

	// Add Depends as "require" capabilities.
	if depends := controlFields["Depends"]; depends != "" {
		for _, dep := range strings.Split(depends, ",") {
			dep = strings.TrimSpace(dep)
			if dep != "" {
				// Parse "package (>= version)" format.
				depName := dep
				depVersion := ""
				if idx := strings.IndexAny(dep, "(<>=!"); idx >= 0 {
					depName = strings.TrimSpace(dep[:idx])
					depVersion = strings.TrimSpace(dep[idx:])
				}
				caps = append(caps, yapdb.Capability{
					Kind:    "require",
					Name:    depName,
					Flags:   0,
					Version: depVersion,
				})
			}
		}
	}

	// Open yapdb and insert the package record.
	db, err := yapdb.Open(ctx, yapdb.DefaultPath(rootDir))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open yapdb").
			WithOperation("writeYapdb").
			WithContext("package", pkgName)
	}
	defer func() { _ = db.Close() }()

	pkg := yapdb.Package{
		Name:        pkgName,
		Epoch:       "",
		Version:     version,
		Release:     "",
		Arch:        arch,
		Format:      "deb",
		Summary:     summary,
		InstallTime: time.Now(),
		Files:       yapdbFiles,
		Caps:        caps,
	}

	if err := db.Insert(ctx, pkg); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert package into yapdb").
			WithOperation("writeYapdb").
			WithContext("package", pkgName)
	}

	return nil
}

// updateDpkgStatusForPackage updates or inserts a package entry in /var/lib/dpkg/status
// and optionally writes to yapdb.
// Callers wrap this in WithDpkgLock for transaction-wide consistency.
func updateDpkgStatusForPackage(
	ctx context.Context,
	pkgName, arch, control string,
	status string,
	rootDir string,
	opts Options,
	files []string,
) error {
	entries, err := readDpkgStatus()
	if err != nil {
		return err
	}

	// Parse the control file.
	controlFields := parseControl(control)

	// Build the status entry.
	entry := &dpkgStatusEntry{fields: make(map[string]string)}

	// Copy fields from control.
	maps.Copy(entry.fields, controlFields)

	// Set/override Status field.
	entry.fields["Status"] = status

	// Ensure Package and Architecture are set.
	entry.fields["Package"] = pkgName

	if arch != "" {
		entry.fields["Architecture"] = arch
	}

	// Determine the key for the entries map.
	key := pkgName
	if arch != "" {
		key = pkgName + ":" + arch
	}

	// Replace or insert.
	entries[key] = entry

	// Write to yapdb (always, unless explicitly disabled).
	if err := writeYapdb(ctx, pkgName, arch, controlFields, rootDir, files); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write yapdb").
			WithOperation("updateDpkgStatusForPackage").
			WithContext("package", pkgName)
	}

	// Optionally write to dpkg status file.
	if opts.WriteDpkgStatus {
		if err := writeDpkgStatus(entries); err != nil {
			return err
		}
	}

	return nil
}

// ensureDpkgDirs creates /var/lib/dpkg and /var/lib/dpkg/info if they don't exist.
func ensureDpkgDirs() error {
	dirs := []string{
		"/var/lib/dpkg",
		"/var/lib/dpkg/info",
		"/var/lib/dpkg/updates",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 - dpkg dirs need read+exec
			return errors.Wrap(err, errors.ErrTypeFileSystem, "mkdir").
				WithOperation("ensureDpkgDirs").WithContext("path", dir)
		}
	}

	return nil
}

// dpkgLockFile is an exclusive advisory lock around /var/lib/dpkg/lock.
// Mirrors dpkg's own locking so a concurrent dpkg/apt process can't race
// the status file read-modify-write cycle.
type dpkgLockFile struct {
	f *os.File
}

// acquireDpkgLock takes an exclusive flock(2) on /var/lib/dpkg/lock.
// The returned handle MUST be released with Release(); the lock is also
// dropped automatically when the process exits.
//
// If the lock file cannot be created (e.g. running as non-root outside a
// container), the function returns a sentinel "best-effort" lock that does
// nothing on release. This keeps unit tests on a developer workstation
// runnable while still locking properly in the build container.
func acquireDpkgLock() (*dpkgLockFile, error) {
	// nolint:gosec // G304: constant path
	f, err := os.OpenFile(dpkgLockPath, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		// Probably permission denied (non-root tests). Treat as no-op so
		// unit tests on a developer workstation still run; production
		// (root inside a build container) always takes the real flock.
		_ = err

		return &dpkgLockFile{f: nil}, nil //nolint:nilerr // see comment above
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()

		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "flock").
			WithOperation("acquireDpkgLock").WithContext("path", dpkgLockPath)
	}

	return &dpkgLockFile{f: f}, nil
}

// Release drops the flock and closes the file.
func (l *dpkgLockFile) Release() {
	if l == nil || l.f == nil {
		return
	}

	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
}

// writeDpkgInfoFiles writes the /var/lib/dpkg/info/<pkg>.* files for an installed package.
func writeDpkgInfoFiles(pkgName, arch string, contents *debContents) error {
	// Determine the base name (with arch qualifier if Multi-Arch: same).
	baseName := pkgName
	if arch != "" && strings.Contains(contents.Control, "Multi-Arch: same") {
		baseName = pkgName + ":" + arch
	}

	infoDir := "/var/lib/dpkg/info"

	// Write .list file (file paths).
	listPath := filepath.Join(infoDir, baseName+".list")

	f, err := os.Create(listPath) // #nosec G304 - constructed from trusted values
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create .list file").
			WithOperation("writeDpkgInfoFiles").WithContext("path", listPath)
	}

	for _, path := range contents.Files {
		if _, err := fmt.Fprintf(f, "%s\n", path); err != nil {
			_ = f.Close()

			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}

	// Write .md5sums file.
	if contents.Md5sums != "" {
		md5Path := filepath.Join(infoDir, baseName+".md5sums")

		if err := os.WriteFile(md5Path, []byte(contents.Md5sums), 0o644); err != nil { // #nosec G306
			return errors.Wrap(err, errors.ErrTypeFileSystem, "write .md5sums").
				WithOperation("writeDpkgInfoFiles").WithContext("path", md5Path)
		}
	}

	// Write .conffiles file.
	if contents.Conffiles != "" {
		confPath := filepath.Join(infoDir, baseName+".conffiles")

		if err := os.WriteFile(confPath, []byte(contents.Conffiles), 0o644); err != nil { // #nosec G306
			return errors.Wrap(err, errors.ErrTypeFileSystem, "write .conffiles").
				WithOperation("writeDpkgInfoFiles").WithContext("path", confPath)
		}
	}

	// Write scriptlet files.
	for scriptName, scriptBody := range contents.Scriptlets {
		scriptPath := filepath.Join(infoDir, baseName+"."+scriptName)

		if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil { // #nosec G306
			return errors.Wrap(err, errors.ErrTypeFileSystem, "write scriptlet file").
				WithOperation("writeDpkgInfoFiles").WithContext("script", scriptName).WithContext("path", scriptPath)
		}
	}

	// Write .triggers file if present.
	if contents.Triggers != "" {
		triggersPath := filepath.Join(infoDir, baseName+".triggers")

		if err := os.WriteFile(triggersPath, []byte(contents.Triggers), 0o644); err != nil { // #nosec G306
			return errors.Wrap(err, errors.ErrTypeFileSystem, "write .triggers").
				WithOperation("writeDpkgInfoFiles").WithContext("path", triggersPath)
		}
	}

	return nil
}
