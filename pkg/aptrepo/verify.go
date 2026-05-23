package aptrepo

// This file implements OpenPGP verification of Debian/Ubuntu InRelease
// (inline clear-signed) and Release+Release.gpg (detached) using the
// ProtonMail/go-crypto fork that the rest of YAP already depends on.
//
// Trust model:
//
//   - When the source declares Signed-By: <path>, exactly that keyring is
//     trusted. A signature outside that keyring is refused.
//   - When the source has no Signed-By directive (legacy one-liner
//     sources.list entries), every key in the standard apt trust paths
//     (/etc/apt/trusted.gpg.d/*.gpg, /etc/apt/trusted.gpg,
//     /usr/share/keyrings/*.gpg) is accepted, mirroring apt's own
//     behaviour.
//   - A keyring file that exists but is empty / unparseable is fatal —
//     the user asked for a specific trust anchor and we cannot provide
//     it. Falling back to "no verification" silently would defeat the
//     point of the directive.
//   - A signature that *exists and fails to verify* is fatal even when
//     AllowUnverifiedRepos is set: bad signatures indicate an attack or
//     a broken mirror, and ignoring them is strictly worse than no
//     verification at all.
//   - When no signature is present (plain Release, no Release.gpg) and
//     the source declares no Signed-By, the caller falls back to the
//     existing AllowUnverifiedRepos opt-in.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
)

// defaultAptKeyringPaths is the set of files/directories apt itself trusts
// for unsigned-by sources, in the same priority order apt uses. Files in
// directories are loaded recursively, but only with the *.gpg / *.asc
// extensions to avoid picking up README files etc.
var defaultAptKeyringPaths = []string{
	"/etc/apt/trusted.gpg.d",
	"/usr/share/keyrings",
	"/etc/apt/keyrings",
	"/etc/apt/trusted.gpg",
}

// ErrNoTrustAnchor is returned when no usable keyring file can be found
// for a source, or when the available keys do not include the signing key.
// The caller decides whether to escalate (strict mode) or fall back to
// the AllowUnverifiedRepos opt-in.
var ErrNoTrustAnchor = errors.New("aptrepo: no usable trust anchor for source")

// ErrUnknownSigner is returned when a signature is present and
// well-formed but was made by a key not in the trust anchor. This is
// distinct from a *bad* signature (corrupted data / wrong key material)
// and is treated the same as ErrNoTrustAnchor for the AllowUnverifiedRepos
// opt-in: the repo hasn't been trusted yet.
var ErrUnknownSigner = errors.New("aptrepo: signature made by unknown entity")

// ErrUnsigned is returned when the fetched Release / InRelease file
// contains no signature data at all. The caller decides how to handle it
// based on AllowUnverifiedRepos.
var ErrUnsigned = errors.New("aptrepo: release file is not signed")

// verifyResult carries the verified, signature-free body of a Release /
// InRelease document along with the entity that signed it (purely for
// logging).
type verifyResult struct {
	body   []byte // hash-manifest text the caller will parse
	signer string // primary identity of the signing key, "" if not available
}

// verifyInRelease verifies an inline clear-signed InRelease document
// against keyring and returns the unwrapped body bytes. The returned
// bytes are LF-terminated (clearsign.Decode normalises CRLF → LF).
func verifyInRelease(data []byte, keyring openpgp.EntityList) (verifyResult, error) {
	block, _ := clearsign.Decode(data)
	if block == nil {
		return verifyResult{}, yaperrors.New(yaperrors.ErrTypeParser, "not a clear-signed message").
			WithOperation("verifyInRelease")
	}

	if block.ArmoredSignature == nil {
		return verifyResult{}, ErrUnsigned
	}

	signer, err := openpgp.CheckDetachedSignature(
		keyring,
		bytes.NewReader(block.Bytes),
		block.ArmoredSignature.Body,
		nil, /* default packet.Config */
	)
	if err != nil {
		return verifyResult{}, wrapPGPError(err)
	}

	return verifyResult{body: block.Plaintext, signer: signerName(signer)}, nil
}

// verifyDetachedRelease verifies a plain Release body against a separate
// armored Release.gpg signature. apt falls back to this pair when
// InRelease is not served by the mirror.
func verifyDetachedRelease(body, signature []byte, keyring openpgp.EntityList) (verifyResult, error) {
	signer, err := openpgp.CheckArmoredDetachedSignature(
		keyring,
		bytes.NewReader(body),
		bytes.NewReader(signature),
		nil,
	)
	if err != nil {
		return verifyResult{}, wrapPGPError(err)
	}

	return verifyResult{body: body, signer: signerName(signer)}, nil
}

// loadKeyringForSource resolves a Signed-By directive (or its absence)
// into an openpgp.EntityList suitable for verification.
//
// signedBy is the raw value from the deb822 / one-liner Signed-By field.
// Filesystem paths are supported; inline armored key blocks
// (a niche deb822 feature) are not. Empty / unset signedBy means "use the
// default apt trust paths".
func loadKeyringForSource(signedBy string) (openpgp.EntityList, error) {
	if signedBy == "" {
		return loadDefaultAptKeyring()
	}

	signedBy = strings.TrimSpace(signedBy)

	info, err := os.Stat(signedBy)
	if err != nil {
		return nil, ErrNoTrustAnchor
	}

	if info.IsDir() {
		return loadKeyringDir(signedBy)
	}

	keys, err := loadKeyringFile(signedBy)
	if err != nil {
		return nil, ErrNoTrustAnchor
	}

	if len(keys) == 0 {
		return nil, ErrNoTrustAnchor
	}

	return keys, nil
}

// loadDefaultAptKeyring walks defaultAptKeyringPaths and returns every
// usable public key found. Errors on individual files are downgraded to
// continue-and-warn — the union of all valid keys is what apt itself
// effectively trusts.
func loadDefaultAptKeyring() (openpgp.EntityList, error) {
	var combined openpgp.EntityList

	for _, p := range defaultAptKeyringPaths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}

		var (
			keys openpgp.EntityList
			lerr error
		)

		if info.IsDir() {
			keys, lerr = loadKeyringDir(p)
		} else {
			keys, lerr = loadKeyringFile(p)
		}

		if lerr != nil {
			// One bad file in the default set isn't fatal — apt itself
			// skips unreadable entries here. Other paths may still
			// provide a usable key.
			continue
		}

		combined = append(combined, keys...)
	}

	if len(combined) == 0 {
		return nil, ErrNoTrustAnchor
	}

	return combined, nil
}

// loadKeyringDir loads every *.gpg / *.asc file in dir (non-recursive,
// matching apt's behaviour for /etc/apt/trusted.gpg.d).
func loadKeyringDir(dir string) (openpgp.EntityList, error) {
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
		if !strings.HasSuffix(name, ".gpg") && !strings.HasSuffix(name, ".asc") {
			continue
		}

		keys, err := loadKeyringFile(filepath.Join(dir, name))
		if err != nil {
			// Skip unreadable entries — match apt's lenient behaviour
			// for the default trust dir.
			continue
		}

		combined = append(combined, keys...)
	}

	return combined, nil
}

// loadKeyringFile reads a single keyring, auto-detecting binary vs
// ASCII-armored format.
func loadKeyringFile(path string) (openpgp.EntityList, error) {
	// default-keyring list; trust boundary is at the sources.list parser.
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, 16<<20)) // 16 MiB cap
	if err != nil {
		return nil, err
	}

	// ASCII armor starts with "-----BEGIN PGP PUBLIC KEY BLOCK-----".
	if bytes.Contains(data, []byte("-----BEGIN PGP")) {
		return openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
	}

	return openpgp.ReadKeyRing(bytes.NewReader(data))
}

// signerName extracts a human-readable identity (e.g. "Ubuntu Archive
// Automatic Signing Key (2018) <[email protected]>") from a
// verified entity, or returns the hex key ID when no UID is present.
func signerName(e *openpgp.Entity) string {
	if e == nil {
		return ""
	}

	for name := range e.Identities {
		return name
	}

	return fmt.Sprintf("%X", e.PrimaryKey.KeyId)
}

// wrapPGPError translates ProtonMail's sentinel + typed errors into a
// stable string that's safe to compare via errors.Is / errors.As in
// callers. The original error is wrapped for full context.
func wrapPGPError(err error) error {
	switch {
	case errors.Is(err, pgperrors.ErrUnknownIssuer):
		// Return the sentinel error directly to preserve identity for errors.Is checks
		return ErrUnknownSigner
	case errors.Is(err, pgperrors.ErrSignatureExpired):
		return yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "signature expired").
			WithOperation("wrapPGPError")
	case errors.Is(err, pgperrors.ErrKeyExpired):
		return yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "signing key expired").
			WithOperation("wrapPGPError")
	case errors.Is(err, pgperrors.ErrKeyRevoked):
		return yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "signing key revoked").
			WithOperation("wrapPGPError")
	}

	// Type-assert SignatureError (string type) for "actual bad signature".
	var sigErr pgperrors.SignatureError
	if errors.As(err, &sigErr) {
		return yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "invalid signature").
			WithOperation("wrapPGPError")
	}

	return yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "verification failed").
		WithOperation("wrapPGPError")
}
