// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/M0Rf30/pkgformat/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

const testKeyName = "yap-test.rsa.pub"

// indexServer serves a one-package APKINDEX.tar.gz for x86_64, signed when
// signer is not nil, and returns the repository list pointing at it.
func indexServer(t *testing.T, signer *pkgapk.Signer) []pkgapk.Repo {
	t.Helper()

	data, err := pkgapk.BuildIndex([]pkgapk.Package{{
		Name: "hello", Version: "1.0-r0", Arch: "x86_64", Checksum: "Q1" + "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		Description: "hello", Size: 100,
	}}, "test", signer)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/main/x86_64/APKINDEX.tar.gz" {
			http.NotFound(w, r)

			return
		}

		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	return []pkgapk.Repo{{URL: srv.URL + "/main"}}
}

// keysDir returns a keys directory trusting the signer's public key.
func keysDir(t *testing.T, signer *pkgapk.Signer) string {
	t.Helper()

	dir := t.TempDir()

	pub, err := signer.PublicKeyPEM()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, testKeyName), pub, 0o600))

	return dir
}

func newSigner(t *testing.T) *pkgapk.Signer {
	t.Helper()

	key, err := pkgapk.GenerateKey(2048)
	require.NoError(t, err)

	signer, err := pkgapk.NewSigner(testKeyName, key, pkgapk.SigRSASHA1)
	require.NoError(t, err)

	return signer
}

func options(repos []pkgapk.Repo, keys string, allowUnverified bool) *apkindex.UpdateOptions {
	return &apkindex.UpdateOptions{
		Repos:           repos,
		Arch:            "x86_64",
		KeysDir:         keys,
		CacheDir:        "",
		AllowUnverified: allowUnverified,
		HTTP:            httpclient.New(httpclient.Options{Retry: httpclient.RetryPolicy{Attempts: 1}}),
	}
}

func TestUpdateAcceptsIndexSignedByTrustedKey(t *testing.T) {
	signer := newSigner(t)
	repos := indexServer(t, signer)

	loaded, err := apkindex.UpdateWithOptions(context.Background(),
		options(repos, keysDir(t, signer), false))
	require.NoError(t, err)

	pkg, ok := loaded.Index.Lookup("hello")
	require.True(t, ok)
	assert.Equal(t, "1.0-r0", pkg.Version)
	assert.Same(t, loaded, apkindex.Load(), "Update must cache the loaded repositories")

	apkindex.SetGlobalRepos(nil)
}

func TestUpdateRefusesUnsignedIndexWhenKeysAreTrusted(t *testing.T) {
	signer := newSigner(t)
	repos := indexServer(t, nil)

	_, err := apkindex.UpdateWithOptions(context.Background(),
		options(repos, keysDir(t, signer), true))
	require.Error(t, err, "keys are present: an unsigned index must not be accepted")
}

func TestUpdateRefusesIndexSignedByUntrustedKey(t *testing.T) {
	repos := indexServer(t, newSigner(t))

	_, err := apkindex.UpdateWithOptions(context.Background(),
		options(repos, keysDir(t, newSigner(t)), true))
	require.Error(t, err)
}

func TestUpdateWithoutKeysRefusesUnlessAllowed(t *testing.T) {
	repos := indexServer(t, nil)
	empty := t.TempDir()

	_, err := apkindex.UpdateWithOptions(context.Background(), options(repos, empty, false))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trusted APK keys")

	_, err = apkindex.UpdateWithOptions(context.Background(), options(repos, filepath.Join(empty, "absent"), false))
	require.Error(t, err)

	loaded, err := apkindex.UpdateWithOptions(context.Background(), options(repos, empty, true))
	require.NoError(t, err)

	_, ok := loaded.Index.Lookup("hello")
	assert.True(t, ok)

	apkindex.SetGlobalRepos(nil)
}

func TestUpdateFailsWhenNoRepositoryLoads(t *testing.T) {
	repos := []pkgapk.Repo{{URL: "http://127.0.0.1:1/main"}}

	_, err := apkindex.UpdateWithOptions(context.Background(), options(repos, t.TempDir(), true))
	require.Error(t, err)
}

func TestInstallUsesCachedRepositories(t *testing.T) {
	apkindex.SetGlobalRepos(&apkindex.Repos{Index: pkgapk.NewIndex()})
	defer apkindex.SetGlobalRepos(nil)

	require.NoError(t, apkindex.Install(context.Background(), nil),
		"an empty request on a cached index needs neither network nor root")
}

func TestInstallUnknownPackageFails(t *testing.T) {
	apkindex.SetGlobalRepos(&apkindex.Repos{Index: pkgapk.NewIndex()})
	defer apkindex.SetGlobalRepos(nil)

	require.Error(t, apkindex.Install(context.Background(), []string{"nonexistent-pkg"}))
}
