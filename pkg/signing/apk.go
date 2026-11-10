// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package signing

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	pkgapk "github.com/M0Rf30/pkgformat/apk"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// apkKeySuffix ends every Alpine public key file name; apk looks a signature's
// key up by the name in the signature entry.
const apkKeySuffix = ".rsa.pub"

// APKSigner signs APK packages with RSA PKCS#1 v1.5 over SHA-1, the signature
// type abuild-sign writes and every apk-tools version verifies. The package
// format work is done by github.com/M0Rf30/pkgformat/apk; this type resolves
// the key and the key name from the signing configuration.
type APKSigner struct {
	cfg    Config
	signer *pkgapk.Signer
}

// NewAPKSigner loads the private key from cfg.KeyPath. The key must be an
// unencrypted PEM-encoded RSA private key (PKCS#1 or PKCS#8) of 2048 to
// 16384 bits.
//
// Encrypted PEM blocks (RFC 1423) are not supported because the underlying
// stdlib helpers (x509.IsEncryptedPEMBlock, x509.DecryptPEMBlock) are
// deprecated and the format is cryptographically broken. Users with
// password-protected keys should re-encode them as unencrypted PEM
// (e.g. via openssl rsa) or use PKCS#8 without password.
func NewAPKSigner(cfg Config) (*APKSigner, error) {
	//nolint:gosec // key path comes from trusted yap config / CLI flag
	keyData, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read key file").
			WithOperation("NewAPKSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	key, err := pkgapk.ParsePrivateKeyPEM(keyData)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			"failed to parse RSA private key").
			WithOperation("NewAPKSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	signer, err := pkgapk.NewSigner(apkKeyName(cfg), key, pkgapk.SigRSASHA1)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			"invalid APK signing key").
			WithOperation("NewAPKSigner").
			WithContext("key_path", cfg.KeyPath).
			WithContext("key_name", cfg.KeyName)
	}

	logger.Debug(i18n.T("logger.signing.debug.loaded_rsa_private_key"),
		"key_path", cfg.KeyPath, "key_size", key.N.BitLen())

	return &APKSigner{cfg: cfg, signer: signer}, nil
}

// apkKeyName returns the public key file name the signature entry carries:
// cfg.KeyName, else the key file's base name without extension, else "yap",
// always ending in ".rsa.pub".
func apkKeyName(cfg Config) string {
	name := cfg.KeyName
	if name == "" {
		base := filepath.Base(cfg.KeyPath)
		name = strings.TrimSuffix(base, filepath.Ext(base))

		if name == "" {
			name = "yap"
		}

		logger.Debug(i18n.T("logger.signing.debug.using_default_key_name"), "key_name", name)
	}

	if !strings.HasSuffix(name, apkKeySuffix) {
		name += apkKeySuffix
	}

	return name
}

// Sign rewrites the APK at artifactPath with a signature segment in front of
// the control segment: .SIGN.RSA.<keyname>.rsa.pub holding the PKCS#1 v1.5
// SHA-1 signature of the control segment.
func (s *APKSigner) Sign(ctx context.Context, artifactPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	err := s.rewrite(artifactPath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to sign APK").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	logger.Info(i18n.T("logger.signing.info.apk_package_signed_successfully"), "artifact_path", artifactPath,
		"key_name", s.signer.KeyName())

	return nil
}

// rewrite signs the package into a sibling temp file and renames it over the
// original, so a failure never leaves a truncated package behind.
func (s *APKSigner) rewrite(artifactPath string) error {
	in, err := os.Open(filepath.Clean(artifactPath))
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	tmpPath := artifactPath + ".tmp"

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec
	if err != nil {
		return err
	}

	err = pkgapk.SignPackage(out, in, info.Size(), s.signer)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	err = os.Rename(tmpPath, artifactPath)
	if err != nil {
		_ = os.Remove(tmpPath)
	}

	return err
}
