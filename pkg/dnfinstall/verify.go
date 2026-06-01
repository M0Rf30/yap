package dnfinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/sassoftware/go-rpmutils"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ErrUnsignedRPM is returned when an RPM has no signature header.
var ErrUnsignedRPM = errors.New("dnfinstall: RPM has no signature")

// ErrUnknownSigner is returned when a signature is present but was made by
// a key not in the trust anchor. This is distinct from a cryptographically
// invalid signature and is treated the same as ErrNoTrustAnchor for the
// AllowUnverifiedRPMs opt-in: the RPM hasn't been trusted yet.
var ErrUnknownSigner = errors.New("dnfinstall: signature made by unknown entity")

// ErrInvalidSignature is returned when a signature is present and
// well-formed but cryptographically invalid (corrupted data, wrong key material).
var ErrInvalidSignature = errors.New("dnfinstall: invalid signature")

// ErrNoTrustAnchor is returned when no usable keyring file can be found
// for RPM signature verification, or when the available keys do not include
// the signing key. The caller decides whether to escalate (strict mode) or
// fall back to the AllowUnverifiedRPMs opt-in.
var ErrNoTrustAnchor = errors.New("dnfinstall: no usable trust anchor for RPM verification")

// verifyRPMSignature checks the OpenPGP signature on the RPM at path.
// If opts.AllowUnverifiedRPMs is true, ALL outcomes (unsigned, unknown signer,
// invalid sig) return nil with a warning logged.
// If opts.KeyringPath is set, loads keys from that path (file OR directory).
// Otherwise loads from /etc/pki/rpm-gpg/.
//
//nolint:gocyclo,cyclop // signature verification has many distinct error paths
func verifyRPMSignature(ctx context.Context, path string, opts Options) error {
	// Check context cancellation.
	if err := ctx.Err(); err != nil {
		return yaperrors.Wrap(err, yaperrors.ErrTypeFileSystem, "context cancelled").
			WithOperation("verifyRPMSignature").
			WithContext("path", path)
	}

	// Resolve trust path.
	trustPath := opts.KeyringPath
	if trustPath == "" {
		trustPath = "/etc/pki/rpm-gpg/"
	}

	// Check if trust path exists.
	_, err := os.Stat(trustPath)
	if err != nil {
		if opts.AllowUnverifiedRPMs {
			logger.Warn(i18n.T("logger.dnfinstall.warn.rpm_keyring_not_found"), "path", path, "keyring", trustPath)

			return nil
		}

		return ErrNoTrustAnchor
	}

	// Load keyring.
	keyring, err := loadRPMKeyring(ctx, trustPath)
	if err != nil {
		if opts.AllowUnverifiedRPMs {
			logger.Warn(i18n.T("logger.dnfinstall.warn.failed_load_rpm_keyring"),
				"path", path, "keyring", trustPath, "error", err)

			return nil
		}

		return err
	}

	if len(keyring) == 0 {
		if opts.AllowUnverifiedRPMs {
			logger.Warn(i18n.T("logger.dnfinstall.warn.no_keys_rpm_keyring"), "path", path, "keyring", trustPath)

			return nil
		}

		return ErrNoTrustAnchor
	}

	// Open RPM file.
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return yaperrors.Wrap(err, yaperrors.ErrTypeFileSystem, "failed to open RPM file").
			WithOperation("verifyRPMSignature").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	// Verify the RPM signature using go-rpmutils.Verify.
	// This checks the PGP signature over the RPM file against the provided keyring.
	_, sigs, err := rpmutils.Verify(f, keyring)
	if err != nil {
		// Determine the error category.
		verifyErr := wrapRPMSignatureError(err)

		if opts.AllowUnverifiedRPMs {
			logger.Warn(i18n.T("logger.dnfinstall.warn.rpm_signature_verification_failed"), "path", path, "error", verifyErr)

			return nil
		}

		return yaperrors.Wrap(verifyErr, yaperrors.ErrTypeValidation, "RPM signature verification failed").
			WithOperation("verifyRPMSignature").
			WithContext("path", path)
	}

	// Verify that we got a signature.
	if len(sigs) == 0 {
		if opts.AllowUnverifiedRPMs {
			logger.Warn(i18n.T("logger.dnfinstall.warn.rpm_unsigned_skipping_verification"), "path", path)
			return nil
		}

		return yaperrors.Wrap(ErrUnsignedRPM, yaperrors.ErrTypeValidation, "RPM is unsigned").
			WithOperation("verifyRPMSignature").
			WithContext("path", path)
	}

	// Log successful verification with signer info if available.
	signerInfo := ""

	if len(sigs) > 0 && sigs[0] != nil {
		if sigs[0].PrimaryName != "" {
			signerInfo = fmt.Sprintf(" by %s", sigs[0].PrimaryName)
		} else {
			signerInfo = fmt.Sprintf(" by %X", sigs[0].KeyId)
		}
	}

	logger.Debug(i18n.T("logger.dnfinstall.debug.rpm_signature_verified"),
		"signer", strings.TrimSpace(strings.TrimPrefix(signerInfo, " by ")),
		"path", filepath.Base(path))

	return nil
}

// loadRPMKeyring returns an openpgp.EntityList from the configured trust path.
// Path may be a single file (one keyring) or a directory containing ASCII-armored
// RPM-GPG-KEY-* files (Fedora convention). Walks the directory non-recursively.
func loadRPMKeyring(ctx context.Context, path string) (openpgp.EntityList, error) {
	// Check context cancellation.
	if err := ctx.Err(); err != nil {
		return nil, yaperrors.Wrap(err, yaperrors.ErrTypeFileSystem, "context cancelled").
			WithOperation("loadRPMKeyring")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return loadRPMKeyringDir(path)
	}

	keys, err := loadRPMKeyringFile(path)
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, ErrNoTrustAnchor
	}

	return keys, nil
}

// loadRPMKeyringDir loads every RPM-GPG-KEY-* / *.gpg / *.asc file in dir
// (non-recursive, matching Fedora's convention for /etc/pki/rpm-gpg/).
func loadRPMKeyringDir(dir string) (openpgp.EntityList, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var combined openpgp.EntityList

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		// Accept Fedora's RPM-GPG-KEY-* pattern, plus standard .gpg / .asc extensions.
		if !strings.HasPrefix(name, "RPM-GPG-KEY-") &&
			!strings.HasSuffix(name, ".gpg") &&
			!strings.HasSuffix(name, ".asc") {
			continue
		}

		keys, err := loadRPMKeyringFile(filepath.Join(dir, name))
		if err != nil {
			// Skip unreadable entries — match Fedora's lenient behaviour
			// for the default trust dir.
			logger.Debug(i18n.T("logger.dnfinstall.debug.skipped_unreadable_keyring_file"),
				"path", filepath.Join(dir, name), "error", err)

			continue
		}

		combined = append(combined, keys...)
	}

	return combined, nil
}

// loadRPMKeyringFile reads a single keyring, auto-detecting binary vs
// ASCII-armored format. Tries armored first, falls back to binary.
func loadRPMKeyringFile(path string) (openpgp.EntityList, error) {
	// trust boundary is at the Options struct.
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, 16<<20)) // 16 MiB cap
	if err != nil {
		return nil, err
	}

	// ASCII armor starts with "-----BEGIN PGP".
	if bytes.Contains(data, []byte("-----BEGIN PGP")) {
		return openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
	}

	return openpgp.ReadKeyRing(bytes.NewReader(data))
}

// wrapRPMSignatureError translates go-rpmutils / ProtonMail's sentinel + typed
// errors into a stable error that's safe to compare via errors.Is / errors.As.
func wrapRPMSignatureError(err error) error {
	if err == nil {
		return nil
	}

	// Check for ProtonMail's sentinel errors.
	switch {
	case errors.Is(err, pgperrors.ErrUnknownIssuer):
		return ErrUnknownSigner
	case errors.Is(err, pgperrors.ErrSignatureExpired):
		return ErrInvalidSignature
	case errors.Is(err, pgperrors.ErrKeyExpired):
		return ErrInvalidSignature
	case errors.Is(err, pgperrors.ErrKeyRevoked):
		return ErrInvalidSignature
	}

	// Type-assert SignatureError (string type) for "actual bad signature".
	var sigErr pgperrors.SignatureError
	if errors.As(err, &sigErr) {
		return ErrInvalidSignature
	}

	// Check for "no signature" patterns in error message.
	errMsg := err.Error()
	if strings.Contains(errMsg, "no signature") ||
		strings.Contains(errMsg, "signature not found") {
		return ErrUnsignedRPM
	}

	// Default to invalid signature for any other error.
	return ErrInvalidSignature
}

// signerName extracts a human-readable identity (e.g. "Fedora Project
// <[email protected]>") from a verified entity, or returns the hex key ID
// when no UID is present.
func signerName(e *openpgp.Entity) string {
	if e == nil {
		return ""
	}

	for name := range e.Identities {
		return name
	}

	return fmt.Sprintf("%X", e.PrimaryKey.KeyId)
}
