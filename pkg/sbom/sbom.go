// Package sbom provides Software Bill of Materials (SBOM) generation
// for YAP packages in CycloneDX and SPDX formats.
package sbom

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Format is the SBOM output format.
type Format string

const (
	// FormatCycloneDX represents CycloneDX 1.5 JSON format.
	FormatCycloneDX Format = "cyclonedx"
	// FormatSPDX represents SPDX 2.3 JSON format.
	FormatSPDX Format = "spdx"
)

// Options controls SBOM generation.
type Options struct {
	// Formats is a list of SBOM formats to generate.
	// Empty list means no SBOM generation.
	Formats []Format
}

// Generate writes one SBOM sidecar per requested format next to artifactPath.
// Returns the list of sidecar paths written. Every requested format is
// attempted; if any of them fails (unknown format, marshal or write error)
// the paths that were written are still returned together with an aggregated
// error. If SBOM generation is disabled (empty Formats), returns an empty
// list and a nil error.
func Generate(pkg *pkgbuild.PKGBUILD, artifactPath string,
	opts Options) ([]string, error) {
	if len(opts.Formats) == 0 {
		return []string{}, nil
	}

	var (
		generatedFiles []string
		errs           []error
	)

	for _, format := range opts.Formats {
		sbomPath, err := generateFormat(pkg, artifactPath, format)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		logger.Debug(i18n.T("logger.sbom.debug.generated_sbom"), "format", format,
			"path", sbomPath)

		generatedFiles = append(generatedFiles, sbomPath)
	}

	if len(errs) > 0 {
		return generatedFiles, yerrors.Wrap(errors.Join(errs...), yerrors.ErrTypePackaging,
			"failed to generate SBOM").
			WithOperation("Generate").
			WithContext("artifact", filepath.Base(artifactPath))
	}

	return generatedFiles, nil
}

// generateFormat renders and writes the SBOM for a single format.
func generateFormat(pkg *pkgbuild.PKGBUILD, artifactPath string, format Format) (string, error) {
	var (
		sbomPath string
		sbomData any
	)

	switch format {
	case FormatCycloneDX:
		sbomPath = artifactPath + ".cdx.json"
		sbomData = generateCycloneDX(pkg)
	case FormatSPDX:
		sbomPath = artifactPath + ".spdx.json"
		sbomData = generateSPDX(pkg)
	default:
		logger.Warn(i18n.T("logger.sbom.warn.unknown_sbom_format"), "format", format)

		return "", yerrors.New(yerrors.ErrTypeValidation, "unknown SBOM format").
			WithOperation("generateFormat").
			WithContext("format", string(format))
	}

	jsonData, err := json.MarshalIndent(sbomData, "", "  ")
	if err != nil {
		logger.Warn(i18n.T("logger.sbom.warn.failed_marshal_sbom_json"), "format", format,
			"artifact", filepath.Base(artifactPath),
			"error", err)

		return "", yerrors.Wrap(err, yerrors.ErrTypePackaging, "failed to marshal SBOM JSON").
			WithOperation("generateFormat").
			WithContext("format", string(format))
	}

	if err := os.WriteFile(sbomPath, jsonData, 0o644); err != nil { //nolint:gosec
		logger.Warn(i18n.T("logger.sbom.warn.failed_write_sbom_file"), "path", sbomPath,
			"error", err)

		return "", yerrors.Wrap(err, yerrors.ErrTypeFileSystem, "failed to write SBOM file").
			WithOperation("generateFormat").
			WithContext("path", sbomPath)
	}

	return sbomPath, nil
}
