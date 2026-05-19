// Package signing provides package signing infrastructure for YAP.
//
// This package establishes the foundation for signing packages across all
// supported formats (APK, DEB, RPM, Pacman). It defines the Signer interface,
// signing configuration, and resolution logic for keys and passphrases.
//
// Phase 0 (this phase) provides infrastructure only. Actual signing
// implementations are added in Phase 3 (APK RSA) and Phase 4 (DEB/RPM/Pacman GPG).
package signing

import (
	"context"
	"fmt"
)

// Format identifies a package format that needs signing.
type Format string

const (
	// FormatAPK represents Alpine Linux APK packages.
	FormatAPK Format = "apk"
	// FormatDEB represents Debian packages.
	FormatDEB Format = "deb"
	// FormatRPM represents Red Hat RPM packages.
	FormatRPM Format = "rpm"
	// FormatPacman represents Arch Linux Pacman packages.
	FormatPacman Format = "pacman"
)

// Algorithm names a signing algorithm/key family.
type Algorithm string

const (
	// AlgorithmRSA uses PKCS#1 v1.5 SHA1, used by APK.
	AlgorithmRSA Algorithm = "rsa"
	// AlgorithmGPG uses OpenPGP, used by DEB/RPM/Pacman.
	AlgorithmGPG Algorithm = "gpg"
)

// Config holds resolved signing configuration for a build.
type Config struct {
	// Enabled indicates whether signing is active.
	Enabled bool
	// KeyPath is the resolved absolute path to the private key
	// (PEM format for RSA, ASCII-armored for GPG).
	KeyPath string
	// Passphrase is the resolved passphrase, may be empty.
	// Use Config.String() / %v formatters never expose this field.
	Passphrase string
	// KeyName is optional, used for APK key naming (e.g., "mykey").
	KeyName string
}

// String implements fmt.Stringer so that accidentally logging or fmt-printing
// a Config never leaks the passphrase. The redacted form preserves enough
// detail (enabled flag, key path, key name, passphrase presence) for
// diagnostics without exposing the secret.
func (c Config) String() string {
	passphrase := "<none>"
	if c.Passphrase != "" {
		passphrase = "<redacted>"
	}

	return fmt.Sprintf("signing.Config{Enabled:%t KeyPath:%q KeyName:%q Passphrase:%s}",
		c.Enabled, c.KeyPath, c.KeyName, passphrase)
}

// Clear zeroes sensitive fields on the Config and marks signing disabled.
// Call after signing completes to drop the passphrase reference promptly.
// The underlying string memory cannot be zeroed (Go strings are immutable),
// but dropping the reference reduces the window in which the passphrase
// remains reachable from goroutine stacks and pinned closures.
func (c *Config) Clear() {
	c.Enabled = false
	c.KeyPath = ""
	c.Passphrase = ""
	c.KeyName = ""
}

// Signer signs a package artifact in place or writes a detached signature.
// Concrete implementations live in this package (RSA for APK, GPG for others).
//
// The behavior depends on the format:
//   - APK: rebuilds the package with a third concatenated gzip stream
//     containing .SIGN.RSA.<keyname>.rsa.pub (Phase 3)
//   - DEB: appends _gpgorigin to the ar archive (dpkg-sig style) (Phase 4)
//   - RPM: writes signature into the RPM lead/header (delegated to rpmpack) (Phase 4)
//   - Pacman: writes detached <artifactPath>.sig (Phase 4)
type Signer interface {
	// Sign signs the artifact at artifactPath.
	Sign(ctx context.Context, artifactPath string) error
}

// NoopSigner is returned when signing is disabled.
// It implements Signer but performs no operation.
type NoopSigner struct{}

// Sign is a no-op implementation of the Signer interface.
func (NoopSigner) Sign(_ context.Context, _ string) error {
	return nil
}
