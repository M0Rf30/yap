// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptrepo

// StripClearsignArmorForTesting exposes stripClearsignArmor for unit tests.
func StripClearsignArmorForTesting(data []byte) []byte {
	return stripClearsignArmor(data)
}

// ParseReleaseForTesting exposes the strip+parse pipeline (sans signature
// verification) for legacy unit tests. New tests should call
// ParseReleaseBodyForTesting directly with pre-stripped content.
func ParseReleaseForTesting(data []byte) (*Release, error) {
	return parseReleaseBody(stripClearsignArmor(data))
}

// ParseReleaseBodyForTesting exposes parseReleaseBody for unit tests.
func ParseReleaseBodyForTesting(body []byte) (*Release, error) {
	return parseReleaseBody(body)
}

// EncodeListFilenameForTesting exposes encodeListFilename for unit tests.
func EncodeListFilenameForTesting(baseURL, suite, relPath string) string {
	return encodeListFilename(baseURL, suite, relPath)
}
