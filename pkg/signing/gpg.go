package signing

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// GPGSigner produces detached OpenPGP signatures for DEB, RPM, and Pacman formats.
// - DEB: writes <package>.deb.asc (ASCII-armored detached signature)
// - RPM: provides signing function for rpmpack.SetPGPSigner (binary signature)
// - Pacman: writes <package>.pkg.tar.zst.sig (binary detached signature)
type GPGSigner struct {
	cfg    Config
	format Format
	entity *openpgp.Entity
}

// NewGPGSigner loads the private key from cfg.KeyPath. The key must be an
// ASCII-armored OpenPGP private key. If cfg.Passphrase is set, it is used
// to decrypt the key.
func NewGPGSigner(cfg Config, format Format) (*GPGSigner, error) {
	// Read the key file
	keyData, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read GPG key file").
			WithOperation("NewGPGSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	// Parse the armored key ring
	keyRing, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyData))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			"failed to parse GPG key").
			WithOperation("NewGPGSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	if len(keyRing) == 0 {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"no keys found in GPG key file").
			WithOperation("NewGPGSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	entity := keyRing[0]

	// Decrypt the key if it's encrypted
	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		err := entity.PrivateKey.Decrypt([]byte(cfg.Passphrase))
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
				"failed to decrypt GPG key with passphrase").
				WithOperation("NewGPGSigner").
				WithContext("key_path", cfg.KeyPath)
		}
	}

	// Verify we have a usable private key
	if entity.PrivateKey == nil {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"GPG key does not contain a private key").
			WithOperation("NewGPGSigner").
			WithContext("key_path", cfg.KeyPath)
	}

	logger.Debug(i18n.T("logger.signing.debug.loaded_gpg_private_key"), "key_path", cfg.KeyPath, "format", string(format),
		"key_id", fmt.Sprintf("%X", entity.PrimaryKey.KeyId))

	return &GPGSigner{
		cfg:    cfg,
		format: format,
		entity: entity,
	}, nil
}

// writeSignatureSidecar writes a signature sidecar file with the given extension.
func (s *GPGSigner) writeSignatureSidecar(
	artifactPath string,
	signatureBuf *bytes.Buffer,
	ext string,
) error {
	sigPath := artifactPath + ext
	if err := os.WriteFile(sigPath, signatureBuf.Bytes(), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write signature file").
			WithOperation("writeSignatureSidecar").
			WithContext("signature_path", sigPath)
	}

	return nil
}

// Sign signs the artifact and writes a detached signature file.
// Output convention by format:
//
//	FormatDEB    -> <artifactPath>.asc (ASCII-armored)
//	FormatRPM    -> returns a signing function for rpmpack.SetPGPSigner
//	FormatPacman -> <artifactPath>.sig (binary, NOT armored)
func (s *GPGSigner) Sign(ctx context.Context, artifactPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Read the artifact
	artifactData, err := os.ReadFile(artifactPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read artifact for signing").
			WithOperation("Sign").
			WithContext("artifact_path", artifactPath)
	}

	// Create detached signature
	signatureBuf := bytes.NewBuffer(nil)

	switch s.format {
	case FormatDEB:
		// DEB: ASCII-armored detached signature
		err = s.createArmoredDetachedSignature(artifactData, signatureBuf)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				"failed to create DEB signature").
				WithOperation("Sign").
				WithContext("artifact_path", artifactPath)
		}

		if err := s.writeSignatureSidecar(artifactPath, signatureBuf, ".asc"); err != nil {
			return err
		}

		logger.Info(i18n.T("logger.signing.info.deb_package_signed_successfully"), "artifact_path", artifactPath,
			"signature_path", artifactPath+".asc")

	case FormatPacman:
		// Pacman: binary detached signature (NOT armored)
		err = s.createBinaryDetachedSignature(artifactData, signatureBuf)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				"failed to create Pacman signature").
				WithOperation("Sign").
				WithContext("artifact_path", artifactPath)
		}

		if err := s.writeSignatureSidecar(artifactPath, signatureBuf, ".sig"); err != nil {
			return err
		}

		logger.Info(i18n.T("logger.signing.info.pacman_package_signed_successfully"), "artifact_path", artifactPath,
			"signature_path", artifactPath+".sig")

	case FormatRPM:
		// RPM: binary detached signature (used by rpmpack.SetPGPSigner)
		err = s.createBinaryDetachedSignature(artifactData, signatureBuf)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				"failed to create RPM signature").
				WithOperation("Sign").
				WithContext("artifact_path", artifactPath)
		}

		if err := s.writeSignatureSidecar(artifactPath, signatureBuf, ".asc"); err != nil {
			return err
		}

		logger.Info(i18n.T("logger.signing.info.rpm_package_signed_successfully"), "artifact_path", artifactPath,
			"signature_path", artifactPath+".asc")

	default:
		return errors.New(errors.ErrTypeConfiguration,
			"unsupported format for GPG signing").
			WithOperation("Sign").
			WithContext("format", string(s.format))
	}

	return nil
}

// createArmoredDetachedSignature creates an ASCII-armored detached signature.
func (s *GPGSigner) createArmoredDetachedSignature(
	data []byte,
	out io.Writer,
) error {
	// Create armor writer
	armorWriter, err := armor.Encode(out, "PGP SIGNATURE", nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to create armor writer").
			WithOperation("createArmoredDetachedSignature")
	}

	defer func() {
		_ = armorWriter.Close()
	}()

	// Create signature
	err = openpgp.DetachSign(armorWriter, s.entity, bytes.NewReader(data), nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to create detached signature").
			WithOperation("createArmoredDetachedSignature")
	}

	return nil
}

// createBinaryDetachedSignature creates a binary detached signature.
func (s *GPGSigner) createBinaryDetachedSignature(
	data []byte,
	out io.Writer,
) error {
	// Create signature
	err := openpgp.DetachSign(out, s.entity, bytes.NewReader(data), nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			"failed to create binary detached signature").
			WithOperation("createBinaryDetachedSignature")
	}

	return nil
}
