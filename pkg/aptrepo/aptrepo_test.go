package aptrepo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
)

// TestStripClearsignArmor tests the armor stripping function.
func TestStripClearsignArmor(t *testing.T) {
	t.Run("plain text (no armor)", func(t *testing.T) {
		input := []byte("Suite: jammy\nCodename: jammy\n")
		result := aptrepo.StripClearsignArmorForTesting(input)
		assert.Equal(t, input, result)
	})

	t.Run("clear-signed message", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Suite: jammy
Codename: jammy

-----BEGIN PGP SIGNATURE-----

iQIzBAEBCAAdFiEE...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		assert.Contains(t, string(result), "Suite: jammy")
		assert.NotContains(t, string(result), "BEGIN PGP SIGNATURE")
	})
}

// TestParseRelease tests Release file parsing.
func TestParseRelease(t *testing.T) {
	t.Run("simple release file", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123def456 1024 main/binary-amd64/Packages.xz
 def789ghi012 2048 main/binary-amd64/Packages.gz
 ghi345jkl678 4096 universe/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "jammy", rel.Suite)
		assert.Equal(t, "jammy", rel.Codename)
		assert.Len(t, rel.SHA256, 3)

		// Check one entry
		entry, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, "abc123def456", entry.Hash)
		assert.Equal(t, int64(1024), entry.Size)
	})
}

// TestEncodeListFilename tests the apt list filename encoding.
func TestEncodeListFilename(t *testing.T) {
	t.Run("ubuntu archive", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_ubuntu_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("ubuntu ports", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://ports.ubuntu.com/ubuntu-ports/",
			"jammy",
			"main/binary-arm64/Packages.gz",
		)
		expected := "ports.ubuntu.com_ubuntu-ports_dists_jammy_main_binary-arm64_Packages.gz"
		assert.Equal(t, expected, result)
	})
}
