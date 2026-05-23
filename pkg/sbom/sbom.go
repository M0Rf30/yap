// Package sbom provides Software Bill of Materials (SBOM) generation
// for YAP packages in CycloneDX and SPDX formats.
package sbom

import (
	"encoding/json"
	"os"
	"path/filepath"

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
// Returns the list of sidecar paths written, or an error if generation fails.
// If SBOM generation is disabled (empty Formats), returns empty list and nil error.
func Generate(pkg *pkgbuild.PKGBUILD, artifactPath string,
	opts Options) ([]string, error) {
	if len(opts.Formats) == 0 {
		return []string{}, nil
	}

	var generatedFiles []string

	for _, format := range opts.Formats {
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
			logger.Warn("Unknown SBOM format",
				"format", format)

			continue
		}

		// Marshal to JSON
		jsonData, err := json.MarshalIndent(sbomData, "", "  ")
		if err != nil {
			logger.Warn("Failed to marshal SBOM to JSON",
				"format", format,
				"artifact", filepath.Base(artifactPath),
				"error", err)

			continue
		}

		// Write to file
		if err := os.WriteFile(sbomPath, jsonData, 0o644); err != nil { //nolint:gosec
			logger.Warn("Failed to write SBOM file",
				"path", sbomPath,
				"error", err)

			continue
		}

		logger.Debug("Generated SBOM",
			"format", format,
			"path", sbomPath)

		generatedFiles = append(generatedFiles, sbomPath)
	}

	return generatedFiles, nil
}
