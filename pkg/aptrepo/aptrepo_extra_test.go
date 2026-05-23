// Package aptrepo_test — extra coverage for aptrepo.go, release.go (fallback
// paths), and verify.go helpers that were not reached by the existing test
// files.
package aptrepo_test

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
)

// ─── SetAllowUnverifiedRepos / AllowUnverifiedRepos ──────────────────────────

// TestSetAllowUnverifiedReposFlag verifies the atomic flag round-trips.
func TestSetAllowUnverifiedReposFlag(t *testing.T) {
	// Restore original state after the test.
	prev := aptrepo.AllowUnverifiedRepos()
	defer aptrepo.SetAllowUnverifiedRepos(prev)

	// Ensure env var doesn't interfere.
	t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", "")

	aptrepo.SetAllowUnverifiedRepos(true)
	assert.True(t, aptrepo.AllowUnverifiedRepos(), "flag set to true → should return true")

	aptrepo.SetAllowUnverifiedRepos(false)
	assert.False(t, aptrepo.AllowUnverifiedRepos(), "flag set to false → should return false")
}

// TestAllowUnverifiedReposEnvVarValues confirms every accepted truthy value
// and a representative set of falsy values.
func TestAllowUnverifiedReposEnvVarValues(t *testing.T) {
	prev := aptrepo.AllowUnverifiedRepos()
	defer aptrepo.SetAllowUnverifiedRepos(prev)

	// Keep the atomic flag off so only the env var drives the result.
	aptrepo.SetAllowUnverifiedRepos(false)

	truthy := []string{"1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On"}
	for _, v := range truthy {
		t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", v)
		assert.True(t, aptrepo.AllowUnverifiedRepos(),
			"env=%q should be truthy", v)
	}

	falsy := []string{"", "0", "false", "FALSE", "no", "NO", "off", "OFF", "garbage", "2"}
	for _, v := range falsy {
		t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", v)
		assert.False(t, aptrepo.AllowUnverifiedRepos(),
			"env=%q should be falsy", v)
	}
}

// TestAllowUnverifiedReposFlagTakesPrecedence confirms the atomic flag wins
// even when the env var is falsy.
func TestAllowUnverifiedReposFlagTakesPrecedence(t *testing.T) {
	prev := aptrepo.AllowUnverifiedRepos()
	defer aptrepo.SetAllowUnverifiedRepos(prev)

	t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", "0")

	aptrepo.SetAllowUnverifiedRepos(true)
	assert.True(t, aptrepo.AllowUnverifiedRepos(),
		"atomic flag=true should win over env=0")
}

// ─── IsVerificationError ─────────────────────────────────────────────────────

// TestIsVerificationError covers all documented cases.
func TestIsVerificationError(t *testing.T) {
	t.Run("nil error → false", func(t *testing.T) {
		assert.False(t, aptrepo.IsVerificationError(nil))
	})

	t.Run("generic error → false", func(t *testing.T) {
		assert.False(t, aptrepo.IsVerificationError(errors.New("network timeout")))
	})

	t.Run("ErrNoTrustAnchor → true", func(t *testing.T) {
		err := aptrepo.ErrNoTrustAnchor
		assert.True(t, aptrepo.IsVerificationError(err))
	})

	t.Run("ErrUnknownSigner → true", func(t *testing.T) {
		err := aptrepo.ErrUnknownSigner
		assert.True(t, aptrepo.IsVerificationError(err))
	})

	t.Run("wrapped ErrNoTrustAnchor → true", func(t *testing.T) {
		wrapped := errors.Join(errors.New("outer"), aptrepo.ErrNoTrustAnchor)
		assert.True(t, aptrepo.IsVerificationError(wrapped))
	})

	t.Run("wrapped ErrUnknownSigner → true", func(t *testing.T) {
		wrapped := errors.Join(errors.New("outer"), aptrepo.ErrUnknownSigner)
		assert.True(t, aptrepo.IsVerificationError(wrapped))
	})

	t.Run("error containing ErrNoTrustAnchor text → true", func(t *testing.T) {
		// IsVerificationError uses string containment, so a fmt.Errorf wrapping
		// the sentinel also matches.
		msg := "aptrepo: " + aptrepo.ErrNoTrustAnchor.Error() + ": extra context"
		assert.True(t, aptrepo.IsVerificationError(errors.New(msg)))
	})

	t.Run("error containing ErrUnknownSigner text → true", func(t *testing.T) {
		msg := "aptrepo: " + aptrepo.ErrUnknownSigner.Error() + ": extra context"
		assert.True(t, aptrepo.IsVerificationError(errors.New(msg)))
	})

	t.Run("ErrUnsigned → false (not a verification error)", func(t *testing.T) {
		// ErrUnsigned means the file has no signature at all — that is a
		// different category from a bad/unknown signature.
		assert.False(t, aptrepo.IsVerificationError(aptrepo.ErrUnsigned))
	})
}

// ─── detectHostDebArch ───────────────────────────────────────────────────────

// TestDetectHostDebArch verifies the function returns a non-empty string and
// that the GOARCH fallback mapping is consistent.
func TestDetectHostDebArch(t *testing.T) {
	arch := aptrepo.DetectHostDebArchForTesting()
	assert.NotEmpty(t, arch, "detectHostDebArch must never return empty string")

	// If /var/lib/dpkg/arch is absent (common in CI / non-Debian hosts),
	// the result must be the GOARCH-derived Debian name.
	if _, err := os.ReadFile("/var/lib/dpkg/arch"); err != nil {
		// Verify the mapping is consistent with runtime.GOARCH.
		expected := goarchToDebArch(runtime.GOARCH)
		assert.Equal(t, expected, arch,
			"GOARCH=%s should map to Debian arch %s", runtime.GOARCH, expected)
	}
}

// goarchToDebArch mirrors the mapping in detectHostDebArch so the test can
// assert the expected value without duplicating the production switch.
func goarchToDebArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armhf"
	case "386":
		return "i386"
	case "ppc64le":
		return "ppc64el"
	case "s390x":
		return "s390x"
	case "riscv64":
		return "riscv64"
	default:
		return goarch
	}
}

// ─── verifyInReleaseOrFallback ───────────────────────────────────────────────

// plainReleaseBody is a minimal Release body without any PGP armor.
const plainReleaseBody = `Suite: jammy
Codename: jammy
SHA256:
 abc123 1024 main/binary-amd64/Packages.xz
`

// TestVerifyInReleaseOrFallback_EmptyKeyringAllowUnverified confirms that
// when there is no trust anchor but allowUnverified=true, the stripped body
// is returned without error.
func TestVerifyInReleaseOrFallback_EmptyKeyringAllowUnverified(t *testing.T) {
	keyringErr := aptrepo.ErrNoTrustAnchor

	body, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		[]byte(plainReleaseBody),
		nil,        // empty keyring
		keyringErr, // the error that explains why it's empty
		true,       // allowUnverified
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Contains(t, string(body), "Suite: jammy",
		"body should contain the release content")
}

// TestVerifyInReleaseOrFallback_EmptyKeyringStrictMode confirms that when
// there is no trust anchor and allowUnverified=false, the keyringErr is
// returned.
func TestVerifyInReleaseOrFallback_EmptyKeyringStrictMode(t *testing.T) {
	keyringErr := aptrepo.ErrNoTrustAnchor

	_, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		[]byte(plainReleaseBody),
		nil,        // empty keyring
		keyringErr, // the error that explains why it's empty
		false,      // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, aptrepo.ErrNoTrustAnchor)
}

// TestVerifyInReleaseOrFallback_PlainBodyAllowUnverified confirms that a
// plain (non-PGP) Release body is returned as-is when allowUnverified=true
// and the keyring is empty.
func TestVerifyInReleaseOrFallback_PlainBodyAllowUnverified(t *testing.T) {
	keyringErr := aptrepo.ErrNoTrustAnchor

	body, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		[]byte(plainReleaseBody),
		nil,
		keyringErr,
		true,
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	// stripClearsignArmor returns the input unchanged for non-armored data.
	assert.Equal(t, plainReleaseBody, string(body))
}

// TestVerifyInReleaseOrFallback_UnsignedAllowUnverified confirms that a
// plain (non-clearsign) body with a populated keyring is treated as a fatal
// format error regardless of allowUnverified — "not a clear-signed message"
// is not the same as ErrUnsigned (which is returned when the clearsign block
// has no ArmoredSignature). The allowUnverified opt-in only bypasses the
// missing-trust-anchor and unknown-signer cases.
func TestVerifyInReleaseOrFallback_UnsignedAllowUnverified(t *testing.T) {
	// Build a non-empty keyring so the "empty keyring" branch is NOT taken.
	e := makeTestEntity(t, "testkey")
	keyring := openpgp.EntityList{e}

	// Plain text is not a clearsign document → verifyInRelease returns
	// "not a clear-signed message", which is NOT ErrUnsigned and is therefore
	// treated as a fatal error even when allowUnverified=true.
	_, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		[]byte(plainReleaseBody),
		keyring,
		nil,
		true, // allowUnverified — does NOT help for a malformed document
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	// URL is stored in context, not in the error message
	var yapErr *yaperrors.YapError
	assert.ErrorAs(t, err, &yapErr, "error should be a YapError")
	if yapErr != nil {
		assert.Equal(t, "https://example.com/ubuntu/", yapErr.Context["url"])
	}
}

// TestVerifyInReleaseOrFallback_UnsignedStrictMode confirms that a plain
// (non-clearsign) body with a populated keyring is also rejected in strict mode.
func TestVerifyInReleaseOrFallback_UnsignedStrictMode(t *testing.T) {
	e := makeTestEntity(t, "testkey")
	keyring := openpgp.EntityList{e}

	_, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		[]byte(plainReleaseBody),
		keyring,
		nil,
		false, // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	// URL is stored in context, not in the error message
	var yapErr *yaperrors.YapError
	assert.ErrorAs(t, err, &yapErr, "error should be a YapError")
	if yapErr != nil {
		assert.Equal(t, "https://example.com/ubuntu/", yapErr.Context["url"])
	}
}

// TestVerifyInReleaseOrFallback_UnknownSignerAllowUnverified confirms that a
// clearsigned body whose signer is not in the keyring is accepted when
// allowUnverified=true.
func TestVerifyInReleaseOrFallback_UnknownSignerAllowUnverified(t *testing.T) {
	signer := makeTestEntity(t, "signer")
	trusted := makeTestEntity(t, "trusted") // different key — not the signer

	armored := clearsignBody(t, signer, []byte(sampleManifest))
	keyring := openpgp.EntityList{trusted}

	body, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		armored,
		keyring,
		nil,
		true, // allowUnverified
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Contains(t, string(body), "Suite: jammy")
}

// TestVerifyInReleaseOrFallback_UnknownSignerStrictMode confirms that a
// clearsigned body whose signer is not in the keyring is rejected when
// allowUnverified=false.
func TestVerifyInReleaseOrFallback_UnknownSignerStrictMode(t *testing.T) {
	signer := makeTestEntity(t, "signer")
	trusted := makeTestEntity(t, "trusted")

	armored := clearsignBody(t, signer, []byte(sampleManifest))
	keyring := openpgp.EntityList{trusted}

	_, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		armored,
		keyring,
		nil,
		false, // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	assert.True(t, aptrepo.IsVerificationError(err),
		"error should be a verification error, got: %v", err)
}

// TestVerifyInReleaseOrFallback_ValidSignature confirms the happy path: a
// correctly signed InRelease is accepted and the body is returned.
func TestVerifyInReleaseOrFallback_ValidSignature(t *testing.T) {
	e := makeTestEntity(t, "trusted-signer")
	armored := clearsignBody(t, e, []byte(sampleManifest))
	keyring := openpgp.EntityList{e}

	body, err := aptrepo.VerifyInReleaseOrFallbackForTesting(
		armored,
		keyring,
		nil,
		false, // strict — should still succeed because signature is valid
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Contains(t, string(body), "Suite: jammy")
}

// ─── verifyDetachedOrFallback ────────────────────────────────────────────────

// TestVerifyDetachedOrFallback_SigErrAllowUnverified confirms that when
// Release.gpg is unavailable (sigErr != nil) and allowUnverified=true, the
// plain body is returned without error.
func TestVerifyDetachedOrFallback_SigErrAllowUnverified(t *testing.T) {
	sigErr := errors.New("404 Not Found")

	body, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		nil,    // no sig bytes
		sigErr, // sig fetch failed
		nil,    // empty keyring
		nil,
		true, // allowUnverified
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Equal(t, plainReleaseBody, string(body))
}

// TestVerifyDetachedOrFallback_SigErrStrictMode confirms that when
// Release.gpg is unavailable and allowUnverified=false, an error is returned.
func TestVerifyDetachedOrFallback_SigErrStrictMode(t *testing.T) {
	sigErr := errors.New("404 Not Found")

	_, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		nil,
		sigErr,
		nil,
		nil,
		false, // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	// URL is stored in context, not in the error message
	var yapErr *yaperrors.YapError
	assert.ErrorAs(t, err, &yapErr, "error should be a YapError")
	if yapErr != nil {
		assert.Equal(t, "https://example.com/ubuntu/", yapErr.Context["url"])
	}
}

// TestVerifyDetachedOrFallback_EmptyKeyringAllowUnverified confirms that
// when sig is present but the keyring is empty and allowUnverified=true,
// the body is returned without error.
func TestVerifyDetachedOrFallback_EmptyKeyringAllowUnverified(t *testing.T) {
	keyringErr := aptrepo.ErrNoTrustAnchor

	body, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		[]byte("some-sig-bytes"), // sig present (non-nil, non-empty)
		nil,                      // no sigErr
		nil,                      // empty keyring
		keyringErr,
		true, // allowUnverified
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Equal(t, plainReleaseBody, string(body))
}

// TestVerifyDetachedOrFallback_EmptyKeyringStrictMode confirms that when
// sig is present but the keyring is empty and allowUnverified=false, the
// keyringErr is returned.
func TestVerifyDetachedOrFallback_EmptyKeyringStrictMode(t *testing.T) {
	keyringErr := aptrepo.ErrNoTrustAnchor

	_, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		[]byte("some-sig-bytes"),
		nil,
		nil,
		keyringErr,
		false, // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, aptrepo.ErrNoTrustAnchor)
}

// TestVerifyDetachedOrFallback_UnknownSignerAllowUnverified confirms that a
// detached signature from an unknown key is accepted when allowUnverified=true.
func TestVerifyDetachedOrFallback_UnknownSignerAllowUnverified(t *testing.T) {
	signer := makeTestEntity(t, "signer")
	trusted := makeTestEntity(t, "trusted")

	// Build a real armored detached signature over the body.
	sig := makeArmoredDetachedSig(t, signer, []byte(plainReleaseBody))
	keyring := openpgp.EntityList{trusted} // trusted ≠ signer

	body, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		sig,
		nil,
		keyring,
		nil,
		true, // allowUnverified
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Equal(t, plainReleaseBody, string(body))
}

// TestVerifyDetachedOrFallback_UnknownSignerStrictMode confirms that a
// detached signature from an unknown key is rejected when allowUnverified=false.
func TestVerifyDetachedOrFallback_UnknownSignerStrictMode(t *testing.T) {
	signer := makeTestEntity(t, "signer")
	trusted := makeTestEntity(t, "trusted")

	sig := makeArmoredDetachedSig(t, signer, []byte(plainReleaseBody))
	keyring := openpgp.EntityList{trusted}

	_, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		sig,
		nil,
		keyring,
		nil,
		false, // strict
		"https://example.com/ubuntu/",
	)

	require.Error(t, err)
	assert.True(t, aptrepo.IsVerificationError(err),
		"error should be a verification error, got: %v", err)
}

// TestVerifyDetachedOrFallback_ValidSignature confirms the happy path: a
// correctly signed Release+Release.gpg pair is accepted.
func TestVerifyDetachedOrFallback_ValidSignature(t *testing.T) {
	e := makeTestEntity(t, "trusted-signer")
	sig := makeArmoredDetachedSig(t, e, []byte(plainReleaseBody))
	keyring := openpgp.EntityList{e}

	body, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		[]byte(plainReleaseBody),
		sig,
		nil,
		keyring,
		nil,
		false, // strict — should still succeed
		"https://example.com/ubuntu/",
	)

	require.NoError(t, err)
	assert.Contains(t, string(body), "Suite: jammy")
}

// TestVerifyDetachedOrFallback_BadSignatureAlwaysFatal confirms that a
// corrupted/invalid signature is rejected even when allowUnverified=true.
func TestVerifyDetachedOrFallback_BadSignatureAlwaysFatal(t *testing.T) {
	e := makeTestEntity(t, "trusted-signer")
	keyring := openpgp.EntityList{e}

	// Corrupt the body after signing so the signature no longer matches.
	originalBody := []byte(plainReleaseBody)
	sig := makeArmoredDetachedSig(t, e, originalBody)
	corruptedBody := []byte(strings.ReplaceAll(string(originalBody), "jammy", "focal"))

	_, err := aptrepo.VerifyDetachedOrFallbackForTesting(
		corruptedBody,
		sig,
		nil,
		keyring,
		nil,
		true, // allowUnverified — should NOT help for a bad signature
		"https://example.com/ubuntu/",
	)

	require.Error(t, err, "corrupted body should always be rejected")
}

// ─── loadKeyringFile / loadKeyringDir / loadKeyringForSource ─────────────────

// TestLoadKeyringFile_ArmoredASC confirms that an ASCII-armored public key
// file is loaded correctly.
func TestLoadKeyringFile_ArmoredASC(t *testing.T) {
	e := makeTestEntity(t, "test-key")
	dir := t.TempDir()
	path := dir + "/key.asc"

	writeArmoredPublicKey(t, e, path)

	keys, err := aptrepo.LoadKeyringFileForTesting(path)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

// TestLoadKeyringFile_NonExistent confirms that a missing file returns an error.
func TestLoadKeyringFile_NonExistent(t *testing.T) {
	_, err := aptrepo.LoadKeyringFileForTesting("/nonexistent/path/key.gpg")
	require.Error(t, err)
}

// TestLoadKeyringDir_FiltersExtensions confirms that only .gpg and .asc files
// are loaded; other extensions are ignored.
func TestLoadKeyringDir_FiltersExtensions(t *testing.T) {
	e := makeTestEntity(t, "test-key")
	dir := t.TempDir()

	// Write a valid .asc key.
	writeArmoredPublicKey(t, e, dir+"/valid.asc")

	// Write a file with an ignored extension.
	require.NoError(t, os.WriteFile(dir+"/ignored.txt", []byte("not a key"), 0o600))

	// Write a file with no extension.
	require.NoError(t, os.WriteFile(dir+"/noext", []byte("not a key"), 0o600))

	keys, err := aptrepo.LoadKeyringDirForTesting(dir)
	require.NoError(t, err)
	assert.Len(t, keys, 1, "only the .asc file should be loaded")
}

// TestLoadKeyringDir_MultipleKeys confirms that multiple key files in a
// directory are all loaded.
func TestLoadKeyringDir_MultipleKeys(t *testing.T) {
	e1 := makeTestEntity(t, "key-one")
	e2 := makeTestEntity(t, "key-two")
	dir := t.TempDir()

	writeArmoredPublicKey(t, e1, dir+"/key1.asc")
	writeArmoredPublicKey(t, e2, dir+"/key2.asc")

	keys, err := aptrepo.LoadKeyringDirForTesting(dir)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

// TestLoadKeyringDir_EmptyDir confirms that an empty directory returns an
// empty (but non-nil) entity list without error.
func TestLoadKeyringDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	keys, err := aptrepo.LoadKeyringDirForTesting(dir)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// TestLoadKeyringDir_NonExistent confirms that a missing directory returns an error.
func TestLoadKeyringDir_NonExistent(t *testing.T) {
	_, err := aptrepo.LoadKeyringDirForTesting("/nonexistent/keyring/dir")
	require.Error(t, err)
}

// TestLoadKeyringForSource_EmptySignedBy confirms that an empty Signed-By
// value triggers the default apt keyring path (which may or may not have keys
// depending on the host, but must not panic).
func TestLoadKeyringForSource_EmptySignedBy(t *testing.T) {
	// This may return an error on hosts without apt keyrings — that's fine.
	// We just confirm it doesn't panic and returns a consistent result.
	keys, err := aptrepo.LoadKeyringForSourceForTesting("")
	if err != nil {
		assert.ErrorIs(t, err, aptrepo.ErrNoTrustAnchor,
			"empty Signed-By with no default keyrings should return ErrNoTrustAnchor")
	} else {
		assert.NotEmpty(t, keys, "if no error, keys must be non-empty")
	}
}

// TestLoadKeyringForSource_ExplicitFile confirms that a Signed-By pointing to
// a valid key file loads that key.
func TestLoadKeyringForSource_ExplicitFile(t *testing.T) {
	e := makeTestEntity(t, "explicit-key")
	dir := t.TempDir()
	path := dir + "/explicit.asc"

	writeArmoredPublicKey(t, e, path)

	keys, err := aptrepo.LoadKeyringForSourceForTesting(path)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

// TestLoadKeyringForSource_ExplicitDir confirms that a Signed-By pointing to
// a directory loads all keys in that directory.
func TestLoadKeyringForSource_ExplicitDir(t *testing.T) {
	e1 := makeTestEntity(t, "dir-key-one")
	e2 := makeTestEntity(t, "dir-key-two")
	dir := t.TempDir()

	writeArmoredPublicKey(t, e1, dir+"/k1.asc")
	writeArmoredPublicKey(t, e2, dir+"/k2.asc")

	keys, err := aptrepo.LoadKeyringForSourceForTesting(dir)
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

// TestLoadKeyringForSource_NonExistentPath confirms that a Signed-By pointing
// to a non-existent path returns ErrNoTrustAnchor.
func TestLoadKeyringForSource_NonExistentPath(t *testing.T) {
	_, err := aptrepo.LoadKeyringForSourceForTesting("/nonexistent/path/key.gpg")
	require.Error(t, err)
	assert.ErrorIs(t, err, aptrepo.ErrNoTrustAnchor)
}

// TestLoadKeyringForSource_EmptyKeyFile confirms that a Signed-By pointing to
// an empty (zero-byte) key file returns ErrNoTrustAnchor.
func TestLoadKeyringForSource_EmptyKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/empty.asc"

	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	_, err := aptrepo.LoadKeyringForSourceForTesting(path)
	require.Error(t, err)
	assert.ErrorIs(t, err, aptrepo.ErrNoTrustAnchor)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// makeArmoredDetachedSig creates an ASCII-armored detached OpenPGP signature
// over body using entity e. This mirrors what apt mirrors serve as Release.gpg.
func makeArmoredDetachedSig(t *testing.T, e *openpgp.Entity, body []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w, err := armor.Encode(&buf, "PGP SIGNATURE", nil)
	if err != nil {
		t.Fatalf("armor.Encode: %v", err)
	}

	if err := openpgp.DetachSign(w, e, bytes.NewReader(body), nil); err != nil {
		t.Fatalf("DetachSign: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("armor close: %v", err)
	}

	return buf.Bytes()
}
