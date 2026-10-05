// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package gensum_test

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/M0Rf30/yap/v2/pkg/gensum"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}

func TestUpdateChecksums_SameBasenameDifferentSources(t *testing.T) {
	bodies := map[string]string{
		"/a/archive/v1.0.tar.gz": "first source body",
		"/b/archive/v1.0.tar.gz": "second, longer source body",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}

		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	dir := writePKGBUILD(t, `pkgname=test
pkgver=1.0
pkgrel=1
source=("`+server.URL+`/a/archive/v1.0.tar.gz"
        "`+server.URL+`/b/archive/v1.0.tar.gz")
sha256sums=('SKIP' 'SKIP')
`)

	require.NoError(t, gensum.UpdateChecksums(dir))

	result := readPKGBUILD(t, dir)
	assert.Contains(t, result, sha256Hex(bodies["/a/archive/v1.0.tar.gz"]))
	assert.Contains(t, result, sha256Hex(bodies["/b/archive/v1.0.tar.gz"]))
}

func writeLocalSource(t *testing.T, content string) (dir, pkgbuild string) {
	t.Helper()

	dir = writePKGBUILD(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o600))

	return dir, filepath.Join(dir, "PKGBUILD")
}

func TestUpdateChecksums_RewritesDeclaredAlgorithm(t *testing.T) {
	const body = "local source"

	b2 := blake2b.Sum512([]byte(body))
	s512 := sha512.Sum512([]byte(body))

	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"sha512sums", "sha512sums", hex.EncodeToString(s512[:])},
		{"b2sums", "b2sums", hex.EncodeToString(b2[:])},
		{"sha256sums", "sha256sums", sha256Hex(body)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, pkgbuild := writeLocalSource(t, body)
			require.NoError(t, os.WriteFile(pkgbuild, []byte("pkgname=t\nsource=('a.txt')\n"+
				tt.field+"=('SKIP')\n"), 0o600))

			require.NoError(t, gensum.UpdateChecksums(dir))

			result := readPKGBUILD(t, dir)
			assert.Contains(t, result, tt.field+"=('"+tt.want+"')")
			assert.Equal(t, 1, strings.Count(result, "sums=("), "no stray checksum block")
		})
	}
}

func TestUpdateChecksums_DefaultsToSHA256(t *testing.T) {
	dir, pkgbuild := writeLocalSource(t, "x")
	require.NoError(t, os.WriteFile(pkgbuild, []byte("pkgname=t\nsource=('a.txt')\n"), 0o600))

	require.NoError(t, gensum.UpdateChecksums(dir))
	assert.Contains(t, readPKGBUILD(t, dir), "sha256sums=(\n  '"+sha256Hex("x")+"'\n)")
}

func TestUpdateChecksums_InlineCommentsInSourceArray(t *testing.T) {
	dir, pkgbuild := writeLocalSource(t, "x")
	require.NoError(t, os.WriteFile(pkgbuild, []byte(`pkgname=t
source=(
  'a.txt' # main (tarball)
)
sha256sums=(
  'SKIP' # old (stale)
)
`), 0o600))

	require.NoError(t, gensum.UpdateChecksums(dir))
	assert.Contains(t, readPKGBUILD(t, dir), "'"+sha256Hex("x")+"' # old (stale)")
}

func TestParseArrayValues_Comments(t *testing.T) {
	vals := gensum.ParseArrayValuesExported(`source=(
  "a.tar.gz" # main
  'b.patch' # (fix) it
  c#frag
)`)
	assert.Equal(t, []string{"a.tar.gz", "b.patch", "c#frag"}, vals)
}
