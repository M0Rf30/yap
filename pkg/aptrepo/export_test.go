// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptrepo

// StripClearsignArmorForTesting exposes stripClearsignArmor for unit tests.
func StripClearsignArmorForTesting(data []byte) []byte {
	return stripClearsignArmor(data)
}

// ParseReleaseForTesting exposes parseRelease for unit tests.
func ParseReleaseForTesting(data []byte) (*Release, error) {
	return parseRelease(data)
}

// EncodeListFilenameForTesting exposes encodeListFilename for unit tests.
func EncodeListFilenameForTesting(baseURL, suite, relPath string) string {
	return encodeListFilename(baseURL, suite, relPath)
}
