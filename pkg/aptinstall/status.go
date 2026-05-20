package aptinstall

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

const dpkgStatusPath = "/var/lib/dpkg/status"

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
func flushDpkgStatusEntry(st *dpkgParseState, entries map[string]*dpkgStatusEntry) {
	if st.currentEntry == nil || st.currentEntry.fields["Package"] == "" {
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

		return nil, fmt.Errorf("read dpkg status: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	st := dpkgParseState{}

	for scanner.Scan() {
		line := scanner.Text()
		handleDpkgStatusLine(line, &st, entries)
	}

	// Flush last entry.
	flushDpkgStatusEntry(&st, entries)

	return entries, nil
}

// writeDpkgStatus writes the dpkg status database atomically.
func writeDpkgStatus(entries map[string]*dpkgStatusEntry) error {
	// Create backup.
	if err := os.Rename(dpkgStatusPath, dpkgStatusPath+"-old"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup dpkg status: %w", err)
	}

	// Write to temporary file.
	tmpPath := dpkgStatusPath + ".dpkg-tmp"

	f, err := os.Create(tmpPath) // #nosec G304 - constant path
	if err != nil {
		return fmt.Errorf("create temp status file: %w", err)
	}

	defer func() { _ = f.Close() }()

	for _, entry := range entries {
		if err := writeStatusEntry(f, entry); err != nil {
			_ = os.Remove(tmpPath)

			return err
		}

		if _, err := f.WriteString("\n"); err != nil {
			_ = os.Remove(tmpPath)

			return err
		}
	}

	if err := f.Sync(); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("sync status file: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("close status file: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, dpkgStatusPath); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("rename status file: %w", err)
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
			if _, err := fmt.Fprintf(f, "%s: %s\n", field, value); err != nil {
				return err
			}

			written[field] = true
		}
	}

	// Write any remaining fields not in the order list.
	for field, value := range entry.fields {
		if !written[field] {
			if _, err := fmt.Fprintf(f, "%s: %s\n", field, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// updateDpkgStatusForPackage updates or inserts a package entry in /var/lib/dpkg/status.
func updateDpkgStatusForPackage(pkgName, arch, control string, status string) error {
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

	return writeDpkgStatus(entries)
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
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	return nil
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
		return fmt.Errorf("create .list file: %w", err)
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
			return fmt.Errorf("write .md5sums: %w", err)
		}
	}

	// Write .conffiles file.
	if contents.Conffiles != "" {
		confPath := filepath.Join(infoDir, baseName+".conffiles")

		if err := os.WriteFile(confPath, []byte(contents.Conffiles), 0o644); err != nil { // #nosec G306
			return fmt.Errorf("write .conffiles: %w", err)
		}
	}

	// Write scriptlet files.
	for scriptName, scriptBody := range contents.Scriptlets {
		scriptPath := filepath.Join(infoDir, baseName+"."+scriptName)

		if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil { // #nosec G306
			return fmt.Errorf("write .%s: %w", scriptName, err)
		}
	}

	// Write .triggers file if present.
	if contents.Triggers != "" {
		triggersPath := filepath.Join(infoDir, baseName+".triggers")

		if err := os.WriteFile(triggersPath, []byte(contents.Triggers), 0o644); err != nil { // #nosec G306
			return fmt.Errorf("write .triggers: %w", err)
		}
	}

	return nil
}
