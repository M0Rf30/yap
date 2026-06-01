package signing

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/gzip"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// RSASigner signs APK packages with PKCS#1 v1.5 SHA1, matching Alpine abuild-sign.
// The signature is computed over the control.tar.gz bytes and prepended as a
// third gzip stream in the final APK file.
type RSASigner struct {
	cfg Config
	key *rsa.PrivateKey
}

// NewRSASigner loads the private key from cfg.KeyPath. The key must be a
// PEM-encoded RSA private key (PKCS#1 or PKCS#8 unencrypted).
//
// Encrypted PEM blocks (RFC 1423) are not supported because the underlying
// stdlib helpers (x509.IsEncryptedPEMBlock, x509.DecryptPEMBlock) are
// deprecated and the format is cryptographically broken. Users with
// password-protected keys should re-encode them as unencrypted PEM
// (e.g. via openssl rsa) or use PKCS#8 without password.
func NewRSASigner(cfg Config) (*RSASigner, error) {
	//nolint:gosec // key path comes from trusted yap config / CLI flag
	keyData, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read key file").
			WithOperation("NewRSASigner").
			WithContext("key_path", cfg.KeyPath)
	}

	key, err := parseRSAPrivateKey(keyData, cfg.KeyPath)
	if err != nil {
		return nil, err
	}

	logger.Debug(i18n.T("logger.signing.debug.loaded_rsa_private_key"),
		"key_path", cfg.KeyPath, "key_size", key.N.BitLen())

	return &RSASigner{
		cfg: cfg,
		key: key,
	}, nil
}

// parseRSAPrivateKey decodes a PEM-encoded RSA private key. It accepts both
// PKCS#1 ("RSA PRIVATE KEY") and PKCS#8 ("PRIVATE KEY") encodings.
func parseRSAPrivateKey(keyData []byte, keyPath string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"invalid PEM format in key file").
			WithOperation("parseRSAPrivateKey").
			WithContext("key_path", keyPath)
	}

	if pkcs1Key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return pkcs1Key, nil
	}

	pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			"failed to parse private key (tried PKCS#1 and PKCS#8)").
			WithOperation("parseRSAPrivateKey").
			WithContext("key_path", keyPath)
	}

	rsaKey, ok := pkcs8Key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"key is not an RSA private key").
			WithOperation("parseRSAPrivateKey").
			WithContext("key_path", keyPath)
	}

	return rsaKey, nil
}

// Sign reads the APK at artifactPath, computes the RSA signature over the
// control.tar.gz stream, and rewrites the APK with the signature stream
// prepended.
//
// The final APK structure is:
//  1. signature.tar.gz — contains .SIGN.RSA.<keyname>.rsa.pub with signature bytes
//  2. control.tar.gz — unchanged from original
//  3. data.tar.gz — unchanged from original
//
// The signature is computed as PKCS#1 v1.5 SHA1(control.tar.gz bytes).
func (s *RSASigner) Sign(ctx context.Context, artifactPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Read the entire APK file
	apkData, err := os.ReadFile(artifactPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read APK file").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	// Find where the first gzip stream (control.tar.gz) ends
	dataStart, err := extractFirstGzipStream(apkData)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to extract control.tar.gz from APK").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	// The original control.tar.gz is the bytes from 0 to dataStart
	controlTarGzCompressed := apkData[:dataStart]

	// Compute SHA1 signature over control.tar.gz bytes (the compressed bytes)
	hash := sha1.Sum(controlTarGzCompressed) //nolint:gosec

	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA1, hash[:])
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to sign control.tar.gz").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	logger.Debug(i18n.T("logger.signing.debug.computed_apk_signature"), "artifact_path", artifactPath,
		"signature_size", len(signature),
		"key_name", s.cfg.KeyName)

	// Create signature tar stream
	signatureTarGz, err := s.createSignatureTar(signature)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to create signature tar").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	// Write the signed APK: signature.tar.gz + control.tar.gz + data.tar.gz
	signedAPK := bytes.NewBuffer(nil)

	if _, err := signedAPK.Write(signatureTarGz); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write signature to APK").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	if _, err := signedAPK.Write(controlTarGzCompressed); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write control to APK").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	if _, err := signedAPK.Write(apkData[dataStart:]); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write data to APK").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	// Write to temporary file first, then rename (atomic)
	tmpPath := artifactPath + ".tmp"
	if err := os.WriteFile(tmpPath, signedAPK.Bytes(), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write signed APK").
			WithOperation("Sign").
			WithContext("artifact_path", tmpPath)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, artifactPath); err != nil {
		_ = os.Remove(tmpPath) // Best effort cleanup

		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to replace APK with signed version").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	logger.Info(i18n.T("logger.signing.info.apk_package_signed_successfully"), "artifact_path", artifactPath,
		"key_name", s.cfg.KeyName)

	return nil
}

// createSignatureTar creates a gzip-compressed tar archive containing a single
// entry: .SIGN.RSA.<keyname>.rsa.pub with the signature bytes as contents.
func (s *RSASigner) createSignatureTar(signature []byte) ([]byte, error) {
	// Determine key name
	keyName := s.cfg.KeyName
	if keyName == "" {
		// Default: use basename of key file without extension
		keyName = filepath.Base(s.cfg.KeyPath)
		if idx := len(keyName) - len(filepath.Ext(keyName)); idx > 0 {
			keyName = keyName[:idx]
		}

		// Fallback if no extension
		if keyName == "" {
			keyName = "yap"
		}

		logger.Debug(i18n.T("logger.signing.debug.using_default_key_name"), "key_name", keyName)
	}

	// Create tar entry name
	entryName := fmt.Sprintf(".SIGN.RSA.%s.rsa.pub", keyName)

	// Create tar archive in memory
	tarBuf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(tarBuf)

	// Create tar header for signature entry
	hdr := &tar.Header{
		Name:     entryName,
		Size:     int64(len(signature)),
		Mode:     0o644,
		ModTime:  time.Unix(0, 0),
		Uid:      0,
		Gid:      0,
		Uname:    "root",
		Gname:    "root",
		Typeflag: tar.TypeReg,
		Format:   tar.FormatPAX,
	}

	// Add PAX records for reproducibility
	hdr.PAXRecords = map[string]string{
		"mtime": "0",
		"atime": "0",
		"ctime": "0",
	}

	// Write header
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to write signature tar header").
			WithOperation("createSignatureTar").
			WithContext("entry_name", entryName)
	}

	// Write signature bytes
	if _, err := tw.Write(signature); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to write signature to tar").
			WithOperation("createSignatureTar").
			WithContext("entry_name", entryName)
	}

	// Close tar writer
	if err := tw.Close(); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to close tar writer").
			WithOperation("createSignatureTar")
	}

	// Gzip the tar
	gzBuf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(gzBuf)
	gz.ModTime = time.Unix(0, 0)

	if _, err := gz.Write(tarBuf.Bytes()); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to gzip signature tar").
			WithOperation("createSignatureTar")
	}

	if err := gz.Close(); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to close gzip writer").
			WithOperation("createSignatureTar")
	}

	return gzBuf.Bytes(), nil
}

// extractFirstGzipStream returns the byte offset where the second concatenated
// gzip stream begins by decompressing the first stream and tracking how many
// bytes were consumed from the underlying reader.
func extractFirstGzipStream(data []byte) (offset int, err error) {
	reader := bytes.NewReader(data)
	initialLen := reader.Len()

	gz, err := gzip.NewReader(reader)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to create gzip reader").
			WithOperation("extractFirstGzipStream")
	}

	if _, err = io.Copy(io.Discard, gz); err != nil {
		_ = gz.Close()

		return 0, errors.Wrap(err, errors.ErrTypePackaging,
			"failed to decompress gzip stream").
			WithOperation("extractFirstGzipStream")
	}

	if closeErr := gz.Close(); closeErr != nil {
		return 0, errors.Wrap(closeErr, errors.ErrTypePackaging,
			"failed to close gzip reader").
			WithOperation("extractFirstGzipStream")
	}

	offset = initialLen - reader.Len()

	return offset, nil
}
