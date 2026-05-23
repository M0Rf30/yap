package aptinstall_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestWriteStatusEntryBasicFields tests that writeStatusEntry writes fields
// in canonical order (Package, Architecture, Status, etc.) followed by
// extra fields in sorted order.
func TestWriteStatusEntryBasicFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package":      "hello",
		"Version":      "1.0",
		"Architecture": "amd64",
		"Status":       "install ok installed",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Verify fields are present
	if !strings.Contains(content, "Package: hello") {
		t.Error("Package field missing")
	}

	if !strings.Contains(content, "Version: 1.0") {
		t.Error("Version field missing")
	}

	if !strings.Contains(content, "Architecture: amd64") {
		t.Error("Architecture field missing")
	}

	if !strings.Contains(content, "Status: install ok installed") {
		t.Error("Status field missing")
	}

	// Verify canonical order: Package should come before Version
	packageIdx := strings.Index(content, "Package:")

	versionIdx := strings.Index(content, "Version:")
	if packageIdx > versionIdx {
		t.Error("Package should come before Version in canonical order")
	}
}

// TestWriteStatusEntryCanonicalOrder tests that fields are written in
// the expected canonical order.
func TestWriteStatusEntryCanonicalOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package":        "test-pkg",
		"Architecture":   "amd64",
		"Multi-Arch":     "same",
		"Status":         "install ok installed",
		"Priority":       "optional",
		"Section":        "utils",
		"Installed-Size": "100",
		"Maintainer":     "Test User <test@example.com>",
		"Version":        "1.0",
		"Depends":        "libc6",
		"Description":    "Test package",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Verify canonical order by checking indices
	indices := make(map[string]int)

	for _, field := range []string{"Package", "Architecture", "Multi-Arch", "Status", "Priority", "Section", "Installed-Size", "Maintainer", "Version", "Depends", "Description"} {
		idx := strings.Index(content, field+":")
		if idx >= 0 {
			indices[field] = idx
		}
	}

	// Check that Package comes first
	if idx, ok := indices["Package"]; !ok || idx > 0 {
		t.Error("Package should be first field")
	}

	// Check that Architecture comes before Status
	if archIdx, ok := indices["Architecture"]; ok {
		if statusIdx, ok := indices["Status"]; ok {
			if archIdx > statusIdx {
				t.Error("Architecture should come before Status")
			}
		}
	}
}

// TestWriteStatusEntryExtraFieldsSorted tests that fields not in the
// canonical order are written in sorted order.
func TestWriteStatusEntryExtraFieldsSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package":      "test-pkg",
		"Version":      "1.0",
		"Zebra-Field":  "z-value",
		"Apple-Field":  "a-value",
		"Banana-Field": "b-value",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Extra fields should be in sorted order: Apple, Banana, Zebra
	appleIdx := strings.Index(content, "Apple-Field:")
	bananaIdx := strings.Index(content, "Banana-Field:")
	zebraIdx := strings.Index(content, "Zebra-Field:")

	if appleIdx < 0 || bananaIdx < 0 || zebraIdx < 0 {
		t.Fatal("Extra fields not found in output")
	}

	if appleIdx > bananaIdx {
		t.Error("Apple-Field should come before Banana-Field")
	}

	if bananaIdx > zebraIdx {
		t.Error("Banana-Field should come before Zebra-Field")
	}
}

// TestWriteStatusEntryEmptyFields tests that empty fields are written
// as "Field:\n" with no trailing space.
func TestWriteStatusEntryEmptyFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package":   "test-pkg",
		"Conffiles": "",
		"Breaks":    "",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Empty fields should be "Field:\n" with no trailing space
	if !strings.Contains(content, "Conffiles:\n") {
		t.Error("Conffiles should be 'Conffiles:\\n' with no trailing space")
	}

	if !strings.Contains(content, "Breaks:\n") {
		t.Error("Breaks should be 'Breaks:\\n' with no trailing space")
	}
}

// TestWriteStatusEntryMultilineFields tests that multi-line fields are
// written with continuation markers (leading space).
func TestWriteStatusEntryMultilineFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package": "test-pkg",
		"Description": "Short description\n" +
			"This is a longer description.\n" +
			".\n" +
			"With multiple paragraphs.",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Multi-line fields should have continuation markers
	if !strings.Contains(content, "Description: Short description\n") {
		t.Error("Description first line should be 'Description: Short description\\n'")
	}

	if !strings.Contains(content, " This is a longer description.\n") {
		t.Error("Description continuation should have leading space")
	}

	if !strings.Contains(content, " .\n") {
		t.Error("Empty line in description should be ' .\\n'")
	}
}

// TestWriteAllStatusEntriesMultipleEntries tests that multiple entries
// are written in sorted key order, separated by blank lines.
func TestWriteAllStatusEntriesMultipleEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]map[string]string{
		"zebra": {
			"Package": "zebra",
			"Version": "1.0",
		},
		"apple": {
			"Package": "apple",
			"Version": "2.0",
		},
		"banana": {
			"Package": "banana",
			"Version": "3.0",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, data); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Entries should be in sorted order: apple, banana, zebra
	appleIdx := strings.Index(content, "Package: apple")
	bananaIdx := strings.Index(content, "Package: banana")
	zebraIdx := strings.Index(content, "Package: zebra")

	if appleIdx < 0 || bananaIdx < 0 || zebraIdx < 0 {
		t.Fatal("Not all entries found in output")
	}

	if appleIdx > bananaIdx {
		t.Error("apple should come before banana")
	}

	if bananaIdx > zebraIdx {
		t.Error("banana should come before zebra")
	}

	// Entries should be separated by blank lines
	if !strings.Contains(content, "\n\n") {
		t.Error("Entries should be separated by blank lines")
	}
}

// TestWriteAllStatusEntriesSingleEntry tests that a single entry is
// written correctly.
func TestWriteAllStatusEntriesSingleEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]map[string]string{
		"hello": {
			"Package": "hello",
			"Version": "1.0",
			"Status":  "install ok installed",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, data); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	if !strings.Contains(content, "Package: hello") {
		t.Error("Package field missing")
	}

	if !strings.Contains(content, "Version: 1.0") {
		t.Error("Version field missing")
	}

	if !strings.Contains(content, "Status: install ok installed") {
		t.Error("Status field missing")
	}
}

// TestWriteAllStatusEntriesEmpty tests that an empty entries map
// produces an empty file.
func TestWriteAllStatusEntriesEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]map[string]string{}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, data); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Errorf("empty entries should produce empty file, got %d bytes", len(got))
	}
}

// TestReadDpkgStatusFromStringRoundTrip tests that status entries survive
// a read-write-read cycle with all fields preserved.
func TestReadDpkgStatusFromStringRoundTrip(t *testing.T) {
	t.Parallel()

	original := "Package: hello\n" +
		"Version: 1.0\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"Priority: optional\n" +
		"Section: utils\n" +
		"Maintainer: Test User <test@example.com>\n" +
		"Description: A greeting\n" +
		" This is a longer description.\n" +
		" .\n" +
		" With multiple paragraphs.\n" +
		"\n"

	// Parse the original
	entries, err := aptinstall.ReadDpkgStatusFromStringForTesting(original)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("should have 1 entry, got %d", len(entries))
	}

	entry := entries["hello:amd64"]
	if entry == nil {
		t.Fatal("entry not found with key 'hello:amd64'")
	}

	// Verify all fields are preserved
	if entry["Package"] != "hello" {
		t.Error("Package field lost")
	}

	if entry["Version"] != "1.0" {
		t.Error("Version field lost")
	}

	if entry["Architecture"] != "amd64" {
		t.Error("Architecture field lost")
	}

	if entry["Status"] != "install ok installed" {
		t.Error("Status field lost")
	}

	if entry["Priority"] != "optional" {
		t.Error("Priority field lost")
	}

	if entry["Section"] != "utils" {
		t.Error("Section field lost")
	}

	if entry["Maintainer"] != "Test User <test@example.com>" {
		t.Error("Maintainer field lost")
	}

	// Description should preserve the multi-line structure
	if !strings.Contains(entry["Description"], "A greeting") {
		t.Error("Description synopsis lost")
	}

	if !strings.Contains(entry["Description"], "longer description") {
		t.Error("Description continuation lost")
	}

	if !strings.Contains(entry["Description"], "multiple paragraphs") {
		t.Error("Description second paragraph lost")
	}
}

// TestWriteAndReadRoundTrip tests that entries written with
// WriteAllStatusEntriesForTesting can be read back with
// ReadDpkgStatusFromStringForTesting with all fields preserved.
func TestWriteAndReadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Write entries
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	originalData := map[string]map[string]string{
		"hello": {
			"Package":      "hello",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
			"Priority":     "optional",
			"Section":      "utils",
			"Maintainer":   "Test User <test@example.com>",
			"Description":  "A greeting\nThis is a longer description.",
		},
		"world": {
			"Package":      "world",
			"Version":      "2.0",
			"Architecture": "arm64",
			"Status":       "install ok installed",
			"Priority":     "standard",
			"Section":      "libs",
			"Description":  "World library",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, originalData); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back the file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the written content
	readBack, err := aptinstall.ReadDpkgStatusFromStringForTesting(string(content))
	if err != nil {
		t.Fatal(err)
	}

	// Verify all entries are present
	if len(readBack) != 2 {
		t.Errorf("should have 2 entries after round-trip, got %d", len(readBack))
	}

	// Verify hello entry
	helloEntry := readBack["hello:amd64"]
	if helloEntry == nil {
		t.Fatal("hello:amd64 entry not found after round-trip")
	}

	if helloEntry["Package"] != "hello" {
		t.Error("hello Package field lost in round-trip")
	}

	if helloEntry["Version"] != "1.0" {
		t.Error("hello Version field lost in round-trip")
	}

	if helloEntry["Status"] != "install ok installed" {
		t.Error("hello Status field lost in round-trip")
	}

	// Verify world entry
	worldEntry := readBack["world:arm64"]
	if worldEntry == nil {
		t.Fatal("world:arm64 entry not found after round-trip")
	}

	if worldEntry["Package"] != "world" {
		t.Error("world Package field lost in round-trip")
	}

	if worldEntry["Version"] != "2.0" {
		t.Error("world Version field lost in round-trip")
	}

	if worldEntry["Section"] != "libs" {
		t.Error("world Section field lost in round-trip")
	}
}

// TestWriteAndReadRoundTripWithMultilineDescription tests that multi-line
// descriptions survive a write-read round-trip.
func TestWriteAndReadRoundTripWithMultilineDescription(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Write entries with complex multi-line description
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	originalData := map[string]map[string]string{
		"apt": {
			"Package":      "apt",
			"Version":      "2.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
			"Description": "commandline package manager\n" +
				"This package provides commandline tools.\n" +
				".\n" +
				"These include:\n" +
				" * apt-get for retrieval of packages\n" +
				" * apt-cache for searching packages",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, originalData); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back the file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the written content
	readBack, err := aptinstall.ReadDpkgStatusFromStringForTesting(string(content))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the entry
	aptEntry := readBack["apt:amd64"]
	if aptEntry == nil {
		t.Fatal("apt:amd64 entry not found after round-trip")
	}

	// Verify description is preserved
	desc := aptEntry["Description"]
	if !strings.Contains(desc, "commandline package manager") {
		t.Error("Description synopsis lost in round-trip")
	}

	if !strings.Contains(desc, "This package provides commandline tools") {
		t.Error("Description first continuation lost in round-trip")
	}

	if !strings.Contains(desc, "These include") {
		t.Error("Description second paragraph lost in round-trip")
	}

	if !strings.Contains(desc, "apt-get for retrieval") {
		t.Error("Description bullet point lost in round-trip")
	}

	if !strings.Contains(desc, "apt-cache for searching") {
		t.Error("Description second bullet point lost in round-trip")
	}
}

// TestWriteAndReadRoundTripWithArchitecture tests that entries with
// architecture qualifiers survive a round-trip.
func TestWriteAndReadRoundTripWithArchitecture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Write entries with different architectures
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	originalData := map[string]map[string]string{
		"libc6:amd64": {
			"Package":      "libc6",
			"Architecture": "amd64",
			"Version":      "2.31-13",
			"Status":       "install ok installed",
		},
		"libc6:arm64": {
			"Package":      "libc6",
			"Architecture": "arm64",
			"Version":      "2.31-13",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, originalData); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back the file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the written content
	readBack, err := aptinstall.ReadDpkgStatusFromStringForTesting(string(content))
	if err != nil {
		t.Fatal(err)
	}

	// Verify both entries are present
	if len(readBack) != 2 {
		t.Errorf("should have 2 entries after round-trip, got %d", len(readBack))
	}

	// Verify amd64 entry
	amd64Entry := readBack["libc6:amd64"]
	if amd64Entry == nil {
		t.Fatal("libc6:amd64 entry not found after round-trip")
	}

	if amd64Entry["Architecture"] != "amd64" {
		t.Error("amd64 Architecture field lost in round-trip")
	}

	// Verify arm64 entry
	arm64Entry := readBack["libc6:arm64"]
	if arm64Entry == nil {
		t.Fatal("libc6:arm64 entry not found after round-trip")
	}

	if arm64Entry["Architecture"] != "arm64" {
		t.Error("arm64 Architecture field lost in round-trip")
	}
}

// TestWriteAndReadRoundTripWithEmptyFields tests that empty fields
// survive a round-trip.
func TestWriteAndReadRoundTripWithEmptyFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "status")

	// Write entries with empty fields
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	originalData := map[string]map[string]string{
		"hello": {
			"Package":     "hello",
			"Version":     "1.0",
			"Status":      "install ok installed",
			"Conffiles":   "",
			"Breaks":      "",
			"Conflicts":   "",
			"Description": "A greeting",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, originalData); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back the file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the written content
	readBack, err := aptinstall.ReadDpkgStatusFromStringForTesting(string(content))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the entry
	helloEntry := readBack["hello"]
	if helloEntry == nil {
		t.Fatal("hello entry not found after round-trip")
	}

	// Verify empty fields are preserved
	if val, ok := helloEntry["Conffiles"]; !ok || val != "" {
		t.Error("Conffiles empty field lost in round-trip")
	}

	if val, ok := helloEntry["Breaks"]; !ok || val != "" {
		t.Error("Breaks empty field lost in round-trip")
	}

	if val, ok := helloEntry["Conflicts"]; !ok || val != "" {
		t.Error("Conflicts empty field lost in round-trip")
	}
}

// TestWriteStatusEntryWithAllCanonicalFields tests that all canonical
// fields are written in the correct order.
func TestWriteStatusEntryWithAllCanonicalFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]string{
		"Package":        "test-pkg",
		"Architecture":   "amd64",
		"Multi-Arch":     "same",
		"Status":         "install ok installed",
		"Priority":       "optional",
		"Section":        "utils",
		"Installed-Size": "100",
		"Maintainer":     "Test User <test@example.com>",
		"Source":         "test-pkg",
		"Version":        "1.0",
		"Depends":        "libc6",
		"Pre-Depends":    "base-files",
		"Conflicts":      "old-pkg",
		"Breaks":         "broken-pkg",
		"Replaces":       "replaced-pkg",
		"Provides":       "virtual-pkg",
		"Essential":      "no",
		"Description":    "Test package",
	}

	if err := aptinstall.WriteStatusEntryForTesting(f, fields); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)

	// Verify all fields are present
	expectedFields := []string{
		"Package", "Architecture", "Multi-Arch", "Status", "Priority",
		"Section", "Installed-Size", "Maintainer", "Source", "Version",
		"Depends", "Pre-Depends", "Conflicts", "Breaks", "Replaces",
		"Provides", "Essential", "Description",
	}

	for _, field := range expectedFields {
		if !strings.Contains(content, field+":") {
			t.Errorf("Field %s missing from output", field)
		}
	}

	// Verify canonical order by checking indices
	indices := make(map[string]int)

	for _, field := range expectedFields {
		idx := strings.Index(content, field+":")
		if idx >= 0 {
			indices[field] = idx
		}
	}

	// Check Package is first
	if idx, ok := indices["Package"]; !ok || idx > 0 {
		t.Error("Package should be first")
	}

	// Check Architecture comes before Status
	if archIdx, ok := indices["Architecture"]; ok {
		if statusIdx, ok := indices["Status"]; ok {
			if archIdx > statusIdx {
				t.Error("Architecture should come before Status")
			}
		}
	}

	// Check Version comes before Depends
	if versionIdx, ok := indices["Version"]; ok {
		if dependsIdx, ok := indices["Depends"]; ok {
			if versionIdx > dependsIdx {
				t.Error("Version should come before Depends")
			}
		}
	}

	// Check Description is last canonical field
	if descIdx, ok := indices["Description"]; ok {
		for field, idx := range indices {
			if field != "Description" && idx > descIdx {
				t.Errorf("Description should come after %s", field)
			}
		}
	}
}
