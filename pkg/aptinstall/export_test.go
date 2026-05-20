// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptinstall

import "io"

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
