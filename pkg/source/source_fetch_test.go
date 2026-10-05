// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//nolint:testpackage // Internal testing of source package methods
package source

import (
	"context"
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

	"github.com/M0Rf30/yap/v2/pkg/download"
)

func TestSource_validateSource_Algorithms(t *testing.T) {
	t.Parallel()

	content := []byte("hash me")
	b2 := blake2b.Sum512(content)
	s224 := sha256.Sum224(content)
	s256 := sha256.Sum256(content)
	s384 := sha512.Sum384(content)
	s512 := sha512.Sum512(content)

	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"sha224", hex.EncodeToString(s224[:]), false},
		{"sha256", hex.EncodeToString(s256[:]), false},
		{"sha256 uppercase", strings.ToUpper(hex.EncodeToString(s256[:])), false},
		{"sha384", hex.EncodeToString(s384[:]), false},
		{"sha512", hex.EncodeToString(s512[:]), false},
		{"b2sums (blake2b-512)", hex.EncodeToString(b2[:]), false},
		{"wrong sha512-length digest", strings.Repeat("0", 128), true},
		{"wrong sha384-length digest", strings.Repeat("0", 96), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "f")
			require.NoError(t, os.WriteFile(path, content, 0o600))

			err := (&Source{Hash: tt.hash}).validateSource(path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSource_GetContext_Cancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	startDir := t.TempDir()
	src := &Source{
		SourceItemURI: "https://example.invalid/file.tar.gz",
		StartDir:      startDir,
		SrcDir:        t.TempDir(),
		Hash:          skipValue,
	}

	err := src.GetContext(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, filepath.Join(startDir, "file.tar.gz"))
}

// Not parallel: mutates the global download retry budget.
func TestSource_Get_PartialDownloadNotLeftAtFinalPath(t *testing.T) {
	old := download.MaxRetries()

	download.SetMaxRetries(0)

	defer download.SetMaxRetries(old)

	// Advertise more bytes than are sent so the transfer is truncated.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	startDir := t.TempDir()
	src := &Source{
		SourceItemURI: server.URL + "/file.tar.gz",
		StartDir:      startDir,
		SrcDir:        t.TempDir(),
		Hash:          skipValue,
	}

	require.Error(t, src.Get())
	assert.NoFileExists(t, filepath.Join(startDir, "file.tar.gz"),
		"truncated download must not occupy the final path")
}

// Not parallel: mutates the global download retry budget.
func TestSource_Get_CompleteDownloadRenamedIntoPlace(t *testing.T) {
	old := download.MaxRetries()

	download.SetMaxRetries(0)

	defer download.SetMaxRetries(old)

	body := []byte("complete body")
	sum := sha256.Sum256(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	startDir := t.TempDir()
	src := &Source{
		SourceItemURI: server.URL + "/payload.bin",
		StartDir:      startDir,
		SrcDir:        t.TempDir(),
		Hash:          hex.EncodeToString(sum[:]),
	}

	require.NoError(t, src.GetContext(context.Background()))

	got, err := os.ReadFile(filepath.Join(startDir, "payload.bin"))
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.NoFileExists(t, filepath.Join(startDir, "payload.bin"+partSuffix))
}
