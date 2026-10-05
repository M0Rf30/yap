package gensum_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
