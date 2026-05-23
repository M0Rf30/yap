// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptrepo

import "github.com/ProtonMail/go-crypto/openpgp"

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

// VerifyInReleaseOrFallbackForTesting exposes verifyInReleaseOrFallback for unit tests.
func VerifyInReleaseOrFallbackForTesting(data []byte, keyring openpgp.EntityList, keyringErr error, allowUnverified bool, baseURL string) ([]byte, error) {
	return verifyInReleaseOrFallback(data, keyring, keyringErr, allowUnverified, baseURL)
}

// VerifyDetachedOrFallbackForTesting exposes verifyDetachedOrFallback for unit tests.
func VerifyDetachedOrFallbackForTesting(body, sig []byte, sigErr error, keyring openpgp.EntityList, keyringErr error, allowUnverified bool, baseURL string) ([]byte, error) {
	return verifyDetachedOrFallback(body, sig, sigErr, keyring, keyringErr, allowUnverified, baseURL)
}

// DetectHostDebArchForTesting exposes detectHostDebArch for unit tests.
func DetectHostDebArchForTesting() string {
	return detectHostDebArch()
}

// LoadKeyringForSourceForTesting exposes loadKeyringForSource for unit tests.
func LoadKeyringForSourceForTesting(signedBy string) (openpgp.EntityList, error) {
	return loadKeyringForSource(signedBy)
}

// LoadKeyringFileForTesting exposes loadKeyringFile for unit tests.
func LoadKeyringFileForTesting(path string) (openpgp.EntityList, error) {
	return loadKeyringFile(path)
}

// LoadKeyringDirForTesting exposes loadKeyringDir for unit tests.
func LoadKeyringDirForTesting(dir string) (openpgp.EntityList, error) {
	return loadKeyringDir(dir)
}
