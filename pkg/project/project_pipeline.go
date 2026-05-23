// Package project provides multi-package project management and build orchestration.
package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/sbom"
	"github.com/M0Rf30/yap/v2/pkg/signing"
	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
)

// signArtifact signs a built package artifact based on its file extension
// and the project's signing configuration.
func (mpc *MultipleProject) signArtifact(proj *Project, artifactPath string) error {
	format, ok := signingFormatForArtifact(artifactPath)
	if !ok {
		// Unknown extension; nothing to sign
		return nil
	}

	signer, err := signing.NewSigner(format, *proj.Signing)
	if err != nil {
		return yerrors.Wrap(err, yerrors.ErrTypeBuild, "failed to create signer").
			WithOperation("signArtifact").
			WithContext("artifact", artifactPath).
			WithContext("format", string(format))
	}

	logger.Info(i18n.T("logger.signing_artifact"),
		"package", proj.Builder.PKGBUILD.PkgName,
		"artifact", artifactPath,
		"format", string(format))

	if err := signer.Sign(context.Background(), artifactPath); err != nil {
		return yerrors.Wrap(err, yerrors.ErrTypeBuild, "failed to sign artifact").
			WithOperation("signArtifact").
			WithContext("artifact", artifactPath).
			WithContext("format", string(format))
	}

	return nil
}

// signingFormatForArtifact maps a file extension to a signing.Format.
func signingFormatForArtifact(artifactPath string) (signing.Format, bool) {
	lower := strings.ToLower(artifactPath)
	switch {
	case strings.HasSuffix(lower, ".apk"):
		return signing.FormatAPK, true
	case strings.HasSuffix(lower, ".deb"):
		return signing.FormatDEB, true
	case strings.HasSuffix(lower, ".rpm"):
		return signing.FormatRPM, true
	case strings.HasSuffix(lower, ".pkg.tar.zst"),
		strings.HasSuffix(lower, ".pkg.tar.xz"),
		strings.HasSuffix(lower, ".pkg.tar.gz"):
		return signing.FormatPacman, true
	}

	return "", false
}

// generateSBOM generates Software Bill of Materials for a built package.
// It generates SBOM sidecars in the requested format(s) for the given artifact.
func (mpc *MultipleProject) generateSBOM(proj *Project, artifactPath string) error {
	// Parse SBOM format flag
	var formats []sbom.Format

	switch strings.ToLower(mpc.Opts.SBOMFormat) {
	case "cyclonedx":
		formats = []sbom.Format{sbom.FormatCycloneDX}
	case "spdx":
		formats = []sbom.Format{sbom.FormatSPDX}
	case "both", "":
		formats = []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX}
	default:
		return yerrors.New(yerrors.ErrTypeConfiguration,
			fmt.Sprintf("invalid SBOM format: %s", mpc.Opts.SBOMFormat)).
			WithOperation("generateSBOM").
			WithContext("format", mpc.Opts.SBOMFormat)
	}

	opts := sbom.Options{Formats: formats}

	_, err := sbom.Generate(proj.Builder.PKGBUILD, artifactPath, opts)
	if err != nil {
		return yerrors.Wrap(err, yerrors.ErrTypeBuild,
			"failed to generate SBOM").
			WithOperation("generateSBOM").
			WithContext("artifact", artifactPath)
	}

	logger.Debug(i18n.T("logger.sbom_generated"),
		"package", proj.Builder.PKGBUILD.PkgName,
		"artifact", filepath.Base(artifactPath))

	return nil
}

// runPostBuildHooks executes signing and SBOM generation after a successful
// package build. Signing failures abort the build; SBOM failures only warn.
func (mpc *MultipleProject) runPostBuildHooks(proj *Project, artifactPath string) error {
	if proj.Signing != nil && proj.Signing.Enabled && artifactPath != "" {
		if err := mpc.signArtifact(proj, artifactPath); err != nil {
			return err
		}
	}

	if mpc.Opts.SBOM && artifactPath != "" {
		if err := mpc.generateSBOM(proj, artifactPath); err != nil {
			logger.Warn(i18n.T("logger.sbom_generation_failed"),
				"package", proj.Builder.PKGBUILD.PkgName,
				"error", err)
		}
	}

	return nil
}
