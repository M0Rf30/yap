// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package signing_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// generateTestRSAKey returns a PKCS#1 PEM-encoded 2048-bit RSA key.
func generateTestRSAKey(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

// writeAPK builds a small unsigned package at dir/hello.apk.
func writeAPK(t *testing.T, dir string) string {
	t.Helper()

	var buf bytes.Buffer

	_, err := pkgapk.WritePackage(&buf, &pkgapk.Spec{
		Info:  pkgapk.Info{Name: "hello", Version: "1.0-r0", Arch: "x86_64"},
		Files: []pkgapk.File{pkgapk.MemFile("hello", 0o755, []byte("#!/bin/sh\n"))},
	}, pkgapk.WriteOptions{})
	require.NoError(t, err)

	path := filepath.Join(dir, "hello.apk")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	return path
}

func writeKey(t *testing.T, dir, name string, pemData []byte) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, pemData, 0o600))

	return path
}

// verifyAPK checks the package at path with a keyring trusting pubPEM under
// keyName, accepting only SHA-1 signatures.
func verifyAPK(t *testing.T, path, keyName string, key *rsa.PublicKey) error {
	t.Helper()

	ring := pkgapk.NewKeyring()
	require.NoError(t, ring.Add(keyName, key))

	f, err := os.Open(path)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	require.NoError(t, err)

	_, err = pkgapk.VerifyPackage(f, info.Size(), pkgapk.VerifyOptions{
		Keys: ring, Algorithms: []pkgapk.SigAlgorithm{pkgapk.SigRSASHA1},
	})

	return err
}

func privateKeyOf(t *testing.T, pemData []byte) *rsa.PrivateKey {
	t.Helper()

	key, err := pkgapk.ParsePrivateKeyPEM(pemData)
	require.NoError(t, err)

	return key
}

func TestAPKSignerSignsWithSHA1UnderKeyName(t *testing.T) {
	dir := t.TempDir()
	keyPEM := generateTestRSAKey(t)
	keyPath := writeKey(t, dir, "test.key", keyPEM)
	apkPath := writeAPK(t, dir)

	signer, err := signing.NewSigner(signing.FormatAPK, signing.Config{
		Enabled: true, KeyPath: keyPath, KeyName: "mykey",
	})
	require.NoError(t, err)
	require.NoError(t, signer.Sign(context.Background(), apkPath))

	require.NoError(t, verifyAPK(t, apkPath, "mykey.rsa.pub", &privateKeyOf(t, keyPEM).PublicKey),
		"the package must verify as a SHA-1 signature under <keyname>.rsa.pub")
	assert.NoFileExists(t, apkPath+".tmp")

	// The package itself is untouched: it still parses and installs.
	data, err := os.ReadFile(apkPath)
	require.NoError(t, err)

	pkg, err := pkgapk.ParsePackage(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.True(t, pkg.Signed)
	assert.Equal(t, "hello", pkg.Name)
}

func TestAPKSignerDefaultKeyNameFromKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPEM := generateTestRSAKey(t)
	keyPath := writeKey(t, dir, "release.pem", keyPEM)
	apkPath := writeAPK(t, dir)

	signer, err := signing.NewSigner(signing.FormatAPK, signing.Config{Enabled: true, KeyPath: keyPath})
	require.NoError(t, err)
	require.NoError(t, signer.Sign(context.Background(), apkPath))

	require.NoError(t, verifyAPK(t, apkPath, "release.rsa.pub", &privateKeyOf(t, keyPEM).PublicKey))
}

func TestAPKSignerKeyNameWithSuffixIsNotDoubled(t *testing.T) {
	dir := t.TempDir()
	keyPEM := generateTestRSAKey(t)
	keyPath := writeKey(t, dir, "k.key", keyPEM)
	apkPath := writeAPK(t, dir)

	signer, err := signing.NewSigner(signing.FormatAPK, signing.Config{
		Enabled: true, KeyPath: keyPath, KeyName: "alpine-dev-1234.rsa.pub",
	})
	require.NoError(t, err)
	require.NoError(t, signer.Sign(context.Background(), apkPath))

	require.NoError(t, verifyAPK(t, apkPath, "alpine-dev-1234.rsa.pub", &privateKeyOf(t, keyPEM).PublicKey))
}

func TestAPKSignerAcceptsPKCS8Key(t *testing.T) {
	dir := t.TempDir()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	keyPath := writeKey(t, dir, "k8.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	apkPath := writeAPK(t, dir)

	signer, err := signing.NewSigner(signing.FormatAPK, signing.Config{Enabled: true, KeyPath: keyPath, KeyName: "k"})
	require.NoError(t, err)
	require.NoError(t, signer.Sign(context.Background(), apkPath))
	require.NoError(t, verifyAPK(t, apkPath, "k.rsa.pub", &key.PublicKey))
}

func TestNewAPKSignerRejectsBadKeys(t *testing.T) {
	dir := t.TempDir()

	small, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	cases := map[string][]byte{
		"not pem":   []byte("not a key"),
		"too small": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(small)}),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			keyPath := writeKey(t, dir, "bad.key", data)

			_, err := signing.NewAPKSigner(signing.Config{Enabled: true, KeyPath: keyPath})
			require.Error(t, err)
		})
	}

	_, err = signing.NewAPKSigner(signing.Config{Enabled: true, KeyPath: filepath.Join(dir, "absent")})
	require.Error(t, err)
}

func TestAPKSignerSignErrors(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeKey(t, dir, "test.key", generateTestRSAKey(t))

	signer, err := signing.NewAPKSigner(signing.Config{Enabled: true, KeyPath: keyPath, KeyName: "k"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, signer.Sign(ctx, writeAPK(t, dir)), context.Canceled)

	require.Error(t, signer.Sign(context.Background(), filepath.Join(dir, "missing.apk")))

	notAPK := filepath.Join(dir, "bad.apk")
	require.NoError(t, os.WriteFile(notAPK, []byte("garbage"), 0o644))
	require.Error(t, signer.Sign(context.Background(), notAPK))
	assert.NoFileExists(t, notAPK+".tmp")

	// A package that is already signed is refused, not signed twice.
	apkPath := writeAPK(t, dir)
	require.NoError(t, signer.Sign(context.Background(), apkPath))
	require.Error(t, signer.Sign(context.Background(), apkPath))
}
