package signing

import (
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// NewSigner constructs the appropriate concrete Signer for the format.
//
// For FormatAPK, returns an RSASigner (PKCS#1 v1.5 SHA1).
// For FormatDEB, FormatRPM, FormatPacman, returns a GPGSigner (OpenPGP).
//
// If signing is disabled (cfg.Enabled == false), a NoopSigner is returned.
func NewSigner(format Format, cfg Config) (Signer, error) {
	// If signing is disabled, return a no-op signer
	if !cfg.Enabled {
		return NoopSigner{}, nil
	}

	// Validate that we have a key path
	if cfg.KeyPath == "" {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"signing enabled but no key path resolved").
			WithOperation("NewSigner").
			WithContext("format", string(format))
	}

	// APK RSA signing (PKCS#1 v1.5 SHA1)
	if format == FormatAPK {
		return NewRSASigner(cfg)
	}

	// DEB/RPM/Pacman GPG signing (OpenPGP)
	if format == FormatDEB || format == FormatRPM || format == FormatPacman {
		return NewGPGSigner(cfg, format)
	}

	return nil, errors.New(errors.ErrTypeConfiguration,
		"unsupported format for signing").
		WithOperation("NewSigner").
		WithContext("format", string(format))
}
