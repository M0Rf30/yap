// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptinstall

import (
	"io"
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
