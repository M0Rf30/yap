// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//nolint:testpackage // Internal testing of source package methods
package source

import (
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func TestSource_validateSource_HashAlgo(t *testing.T) {
	t.Parallel()

	content := []byte("algo matters")
	b2 := hex.EncodeToString(func() []byte { s := blake2b.Sum512(content); return s[:] }())
	s512 := hex.EncodeToString(func() []byte { s := sha512.Sum512(content); return s[:] }())

	tests := []struct {
		name    string
		algo    string
		hash    string
		wantErr bool
	}{
		{"sha512 matches sha512sums", "sha512sums", s512, false},
		{"b2 matches b2sums", "b2sums", b2, false},
		{"b2 digest rejected under sha512sums", "sha512sums", b2, true},
		{"sha512 digest rejected under b2sums", "b2sums", s512, true},
		{"wrong length for algo", "sha256sums", s512, true},
		{"no algo keeps inference", "", b2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "f")
			require.NoError(t, os.WriteFile(path, content, 0o600))

			err := (&Source{Hash: tt.hash, HashAlgo: tt.algo}).validateSource(path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
