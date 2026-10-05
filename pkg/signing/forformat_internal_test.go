package signing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupKeysDir points HOME at a temp dir and creates the named key files in
// ~/.config/yap/keys. It returns the keys directory.
func setupKeysDir(t *testing.T, names ...string) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	keysDir := filepath.Join(home, ".config", "yap", "keys")
	require.NoError(t, os.MkdirAll(keysDir, 0o700))

	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(keysDir, name), []byte("k"), 0o600))
	}

	return keysDir
}

func TestForFormatDefaultKeyMatchesAlgorithm(t *testing.T) {
	keysDir := setupKeysDir(t, "default.rsa", "default.gpg")
	t.Setenv("YAP_SIGN_KEY", "")

	generic, err := ResolveGeneric("", "", "", "")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(keysDir, "default.rsa"), generic.KeyPath,
		"generic resolution prefers default.rsa")

	apk, err := ForFormat(FormatAPK, generic)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(keysDir, "default.rsa"), apk.KeyPath)

	for _, f := range []Format{FormatDEB, FormatRPM, FormatPacman} {
		got, err := ForFormat(f, generic)
		require.NoError(t, err)
		assert.True(t, got.Enabled)
		assert.Equal(t, filepath.Join(keysDir, "default.gpg"), got.KeyPath, string(f))
	}
}

func TestForFormatPrefersFormatSpecificFile(t *testing.T) {
	keysDir := setupKeysDir(t, "default.rsa", "default.gpg", "deb.gpg")
	t.Setenv("YAP_SIGN_KEY", "")

	generic, err := ResolveGeneric("", "", "", "")
	require.NoError(t, err)

	deb, err := ForFormat(FormatDEB, generic)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(keysDir, "deb.gpg"), deb.KeyPath)
}

func TestForFormatDisablesWhenNoMatchingDefault(t *testing.T) {
	setupKeysDir(t, "default.rsa")
	t.Setenv("YAP_SIGN_KEY", "")

	generic, err := ResolveGeneric("", "secret", "", "")
	require.NoError(t, err)

	got, err := ForFormat(FormatDEB, generic)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Empty(t, got.Passphrase)

	signer, err := NewSigner(FormatDEB, generic)
	require.NoError(t, err)
	assert.IsType(t, NoopSigner{}, signer)
}

func TestForFormatHonoursFormatEnvVars(t *testing.T) {
	setupKeysDir(t, "default.rsa")

	tmp := t.TempDir()
	globalKey := filepath.Join(tmp, "global.key")
	debKey := filepath.Join(tmp, "deb.key")

	for _, p := range []string{globalKey, debKey} {
		require.NoError(t, os.WriteFile(p, []byte("k"), 0o600))
	}

	t.Setenv("YAP_SIGN_KEY", globalKey)
	t.Setenv("YAP_SIGN_PASSPHRASE", "global-pass")
	t.Setenv("YAP_DEB_KEY", debKey)
	t.Setenv("YAP_DEB_PASSPHRASE", "deb-pass")

	generic, err := ResolveGeneric("", "", "", "")
	require.NoError(t, err)
	require.Equal(t, globalKey, generic.KeyPath)

	deb, err := ForFormat(FormatDEB, generic)
	require.NoError(t, err)
	assert.Equal(t, debKey, deb.KeyPath)
	assert.Equal(t, "deb-pass", deb.Passphrase)

	// Formats without a format-specific variable keep the global key.
	rpm, err := ForFormat(FormatRPM, generic)
	require.NoError(t, err)
	assert.Equal(t, globalKey, rpm.KeyPath)
	assert.Equal(t, "global-pass", rpm.Passphrase)
}

func TestForFormatKeepsExplicitFlagKey(t *testing.T) {
	setupKeysDir(t, "default.gpg")

	tmp := t.TempDir()
	flagKey := filepath.Join(tmp, "flag.key")
	debKey := filepath.Join(tmp, "deb.key")

	for _, p := range []string{flagKey, debKey} {
		require.NoError(t, os.WriteFile(p, []byte("k"), 0o600))
	}

	t.Setenv("YAP_SIGN_KEY", "")
	t.Setenv("YAP_DEB_KEY", debKey)
	t.Setenv("YAP_DEB_PASSPHRASE", "deb-pass")

	generic, err := ResolveGeneric(flagKey, "flag-pass", "", "")
	require.NoError(t, err)

	got, err := ForFormat(FormatDEB, generic)
	require.NoError(t, err)
	assert.Equal(t, flagKey, got.KeyPath, "CLI flag outranks format env")
	assert.Equal(t, "flag-pass", got.Passphrase)
}

func TestForFormatDisabledConfigUnchanged(t *testing.T) {
	got, err := ForFormat(FormatDEB, Config{})
	require.NoError(t, err)
	assert.Equal(t, Config{}, got)
}

func TestForFormatInvalidFormatEnvKeyErrors(t *testing.T) {
	setupKeysDir(t, "default.gpg")
	t.Setenv("YAP_SIGN_KEY", "")
	t.Setenv("YAP_DEB_KEY", filepath.Join(t.TempDir(), "missing.key"))

	generic, err := ResolveGeneric("", "", "", "")
	require.NoError(t, err)

	_, err = NewSigner(FormatDEB, generic)
	require.Error(t, err)
}
