package aptrepo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
)

// TestParseReleaseBody tests the parseReleaseBody function via ParseReleaseBodyForTesting.
func TestParseReleaseBody(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		rel, err := aptrepo.ParseReleaseBodyForTesting([]byte(""))
		require.NoError(t, err)
		assert.Equal(t, "", rel.Codename)
		assert.Equal(t, "", rel.Suite)
		assert.Empty(t, rel.SHA256)
	})

	t.Run("body with only metadata (no SHA256)", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
Date: Fri, 21 May 2026 12:00:00 UTC
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "jammy", rel.Codename)
		assert.Equal(t, "jammy", rel.Suite)
		assert.Empty(t, rel.SHA256)
	})

	t.Run("body with SHA256 section", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123def456 1024 main/binary-amd64/Packages.xz
 def789ghi012 2048 main/binary-amd64/Packages.gz
 ghi345jkl678 4096 universe/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "jammy", rel.Codename)
		assert.Equal(t, "jammy", rel.Suite)
		assert.Len(t, rel.SHA256, 3)

		// Verify individual entries
		entry, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, "abc123def456", entry.Hash)
		assert.Equal(t, int64(1024), entry.Size)

		entry, ok = rel.SHA256["main/binary-amd64/Packages.gz"]
		assert.True(t, ok)
		assert.Equal(t, "def789ghi012", entry.Hash)
		assert.Equal(t, int64(2048), entry.Size)

		entry, ok = rel.SHA256["universe/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, "ghi345jkl678", entry.Hash)
		assert.Equal(t, int64(4096), entry.Size)
	})

	t.Run("malformed SHA256 entries (< 3 fields)", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123def456 1024 main/binary-amd64/Packages.xz
 malformed_entry_only_two_fields
 def789ghi012 2048 main/binary-amd64/Packages.gz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		// Malformed entry should be skipped, but valid ones should be parsed
		assert.Len(t, rel.SHA256, 2)
		_, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		_, ok = rel.SHA256["main/binary-amd64/Packages.gz"]
		assert.True(t, ok)
	})

	t.Run("SHA256 section with invalid size (non-numeric)", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123def456 notanumber main/binary-amd64/Packages.xz
 def789ghi012 2048 main/binary-amd64/Packages.gz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		// Invalid size should be parsed as 0 (strconv.ParseInt returns 0 on error)
		entry, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, int64(0), entry.Size)

		entry, ok = rel.SHA256["main/binary-amd64/Packages.gz"]
		assert.True(t, ok)
		assert.Equal(t, int64(2048), entry.Size)
	})

	t.Run("multiple SHA256 entries with various architectures", func(t *testing.T) {
		input := []byte(`Suite: focal
Codename: focal
SHA256:
 hash1 100 main/binary-amd64/Packages.xz
 hash2 200 main/binary-arm64/Packages.xz
 hash3 300 main/binary-i386/Packages.xz
 hash4 400 universe/binary-amd64/Packages.xz
 hash5 500 multiverse/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Len(t, rel.SHA256, 5)
		assert.Equal(t, "focal", rel.Codename)
		assert.Equal(t, "focal", rel.Suite)
	})

	t.Run("codename and suite can differ", func(t *testing.T) {
		input := []byte(`Suite: stable
Codename: bookworm
SHA256:
 abc123 1024 main/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "bookworm", rel.Codename)
		assert.Equal(t, "stable", rel.Suite)
	})

	t.Run("blank lines are ignored", func(t *testing.T) {
		input := []byte(`Suite: jammy

Codename: jammy

SHA256:
 abc123 1024 main/binary-amd64/Packages.xz

`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "jammy", rel.Codename)
		assert.Equal(t, "jammy", rel.Suite)
		assert.Len(t, rel.SHA256, 1)
	})

	t.Run("continuation lines with tabs", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
	abc123 1024 main/binary-amd64/Packages.xz
	def456 2048 main/binary-amd64/Packages.gz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Len(t, rel.SHA256, 2)
	})

	t.Run("non-SHA256 continuation lines are skipped", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
Description:
 This is a multi-line description
 that should be ignored
SHA256:
 abc123 1024 main/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Len(t, rel.SHA256, 1)
	})

	t.Run("case-sensitive field matching", func(t *testing.T) {
		input := []byte(`suite: jammy
codename: jammy
sha256:
 abc123 1024 main/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		// Lowercase field names should not match
		assert.Equal(t, "", rel.Codename)
		assert.Equal(t, "", rel.Suite)
		assert.Empty(t, rel.SHA256)
	})

	t.Run("large file sizes", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123 9223372036854775807 main/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)

		entry, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, int64(9223372036854775807), entry.Size) // max int64
	})

	t.Run("SHA256 section followed by other fields", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123 1024 main/binary-amd64/Packages.xz
Date: Fri, 21 May 2026 12:00:00 UTC
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Len(t, rel.SHA256, 1)
		assert.Equal(t, "jammy", rel.Suite)
	})

	t.Run("multiple spaces between fields", func(t *testing.T) {
		input := []byte(`Suite: jammy
Codename: jammy
SHA256:
 abc123def456    1024    main/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)

		entry, ok := rel.SHA256["main/binary-amd64/Packages.xz"]
		assert.True(t, ok)
		assert.Equal(t, "abc123def456", entry.Hash)
		assert.Equal(t, int64(1024), entry.Size)
	})
}

// TestStripClearsignArmorEdgeCases tests edge cases for stripClearsignArmor.
func TestStripClearsignArmorEdgeCases(t *testing.T) {
	t.Run("no signature block", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Suite: jammy
Codename: jammy
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		// The function returns everything from after headers to end of input
		assert.Equal(t, []byte("Suite: jammy\nCodename: jammy\n"), result)
	})

	t.Run("CRLF line endings", func(t *testing.T) {
		input := []byte("-----BEGIN PGP SIGNED MESSAGE-----\r\nHash: SHA256\r\n\r\nSuite: jammy\r\nCodename: jammy\r\n-----BEGIN PGP SIGNATURE-----\r\niQIz...\r\n-----END PGP SIGNATURE-----\r\n")
		result := aptrepo.StripClearsignArmorForTesting(input)
		assert.Contains(t, string(result), "Suite: jammy")
		assert.NotContains(t, string(result), "BEGIN PGP SIGNATURE")
	})

	t.Run("multiple armor headers", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256
Charset: UTF-8

Suite: jammy
Codename: jammy

-----BEGIN PGP SIGNATURE-----
iQIz...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		assert.Equal(t, []byte("Suite: jammy\nCodename: jammy"), result)
	})

	t.Run("armor marker in body (should not confuse parser)", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Suite: jammy
This line mentions -----BEGIN PGP SIGNATURE----- in the body
Codename: jammy

-----BEGIN PGP SIGNATURE-----
iQIz...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		// Should stop at the first "-----BEGIN PGP SIGNATURE-----"
		assert.NotContains(t, string(result), "iQIz")
	})

	t.Run("trailing whitespace after body", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Suite: jammy
Codename: jammy
   
-----BEGIN PGP SIGNATURE-----
iQIz...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		// Trailing whitespace should be trimmed
		assert.Equal(t, []byte("Suite: jammy\nCodename: jammy"), result)
	})

	t.Run("empty body between headers and signature", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

-----BEGIN PGP SIGNATURE-----
iQIz...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		assert.Equal(t, []byte(""), result)
	})

	t.Run("no blank line after headers (malformed)", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256
Suite: jammy
Codename: jammy
-----BEGIN PGP SIGNATURE-----
iQIz...
-----END PGP SIGNATURE-----
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		// Should return the data as-is since there's no blank line after headers
		assert.Equal(t, input, result)
	})

	t.Run("only BEGIN marker, no END marker", func(t *testing.T) {
		input := []byte(`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Suite: jammy
Codename: jammy
`)
		result := aptrepo.StripClearsignArmorForTesting(input)
		// Should return everything from start to end since no signature block
		assert.Equal(t, []byte("Suite: jammy\nCodename: jammy\n"), result)
	})
}

// TestEncodeListFilenameEdgeCases tests edge cases for encodeListFilename.
func TestEncodeListFilenameEdgeCases(t *testing.T) {
	t.Run("invalid URL (no scheme)", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"not a valid url at all",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		// url.Parse doesn't fail on invalid URLs, it just parses them
		// "not a valid url at all" gets parsed with empty Host and Path
		assert.NotEmpty(t, result)
		assert.Contains(t, result, "dists_jammy")
	})

	t.Run("URL without path", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("URL with trailing slash", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("URL with multiple path segments", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://example.com/debian/archive/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "example.com_debian_archive_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("URL with port number", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com:8080/ubuntu/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com:8080_ubuntu_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("different suite names", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu/",
			"focal",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_ubuntu_dists_focal_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("different component paths", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu/",
			"jammy",
			"universe/binary-arm64/Packages.gz",
		)
		expected := "archive.ubuntu.com_ubuntu_dists_jammy_universe_binary-arm64_Packages.gz"
		assert.Equal(t, expected, result)
	})

	t.Run("empty suite", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu/",
			"",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_ubuntu_dists__main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("empty relPath", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu/",
			"jammy",
			"",
		)
		expected := "archive.ubuntu.com_ubuntu_dists_jammy_"
		assert.Equal(t, expected, result)
	})

	t.Run("http (not https)", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"http://archive.ubuntu.com/ubuntu/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "archive.ubuntu.com_ubuntu_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("complex path with many segments", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://mirror.example.com/debian/archive/releases/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		expected := "mirror.example.com_debian_archive_releases_dists_jammy_main_binary-amd64_Packages.xz"
		assert.Equal(t, expected, result)
	})

	t.Run("special characters in path (URL-encoded)", func(t *testing.T) {
		result := aptrepo.EncodeListFilenameForTesting(
			"https://archive.ubuntu.com/ubuntu%20archive/",
			"jammy",
			"main/binary-amd64/Packages.xz",
		)
		// URL parsing should handle the encoded space
		assert.NotEmpty(t, result)
		assert.Contains(t, result, "dists_jammy")
	})
}

// TestParseReleaseBodyWithRealWorldData tests with realistic Release file content.
func TestParseReleaseBodyWithRealWorldData(t *testing.T) {
	t.Run("ubuntu jammy release file structure", func(t *testing.T) {
		input := []byte(`Origin: Ubuntu
Label: Ubuntu
Suite: jammy
Version: 22.04
Codename: jammy
Date: Fri, 21 May 2026 12:00:00 UTC
Architectures: amd64 arm64 armhf i386 ppc64el riscv64 s390x
Components: main restricted universe multiverse
Description: Ubuntu 22.04 LTS
SHA256:
 1234567890abcdef 1024 main/binary-amd64/Packages.xz
 abcdef1234567890 2048 main/binary-amd64/Packages.gz
 fedcba0987654321 4096 main/binary-arm64/Packages.xz
 0987654321fedcba 8192 restricted/binary-amd64/Packages.xz
 1111111111111111 16384 universe/binary-amd64/Packages.xz
 2222222222222222 32768 multiverse/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "jammy", rel.Codename)
		assert.Equal(t, "jammy", rel.Suite)
		assert.Len(t, rel.SHA256, 6)
	})

	t.Run("debian bookworm release file structure", func(t *testing.T) {
		input := []byte(`Origin: Debian
Label: Debian
Suite: stable
Version: 12.0
Codename: bookworm
Date: Fri, 21 May 2026 12:00:00 UTC
Architectures: amd64 arm64 armel armhf i386 mips64el mipsel ppc64el s390x
Components: main contrib non-free non-free-firmware
Description: Debian 12 (bookworm)
SHA256:
 aaaaaaaaaaaaaaaa 1024 main/binary-amd64/Packages.xz
 bbbbbbbbbbbbbbbb 2048 contrib/binary-amd64/Packages.xz
 cccccccccccccccc 4096 non-free/binary-amd64/Packages.xz
 dddddddddddddddd 8192 non-free-firmware/binary-amd64/Packages.xz
`)
		rel, err := aptrepo.ParseReleaseBodyForTesting(input)
		require.NoError(t, err)
		assert.Equal(t, "bookworm", rel.Codename)
		assert.Equal(t, "stable", rel.Suite)
		assert.Len(t, rel.SHA256, 4)
	})
}
