package sbom_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/sbom"
)

// TestSBOMIntegration tests the complete SBOM generation workflow.
func TestSBOMIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a realistic package
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "myapp",
		PkgVer:      "2.1.0",
		PkgRel:      "3",
		PkgDesc:     "A sample application for testing SBOM generation",
		License:     []string{"MIT", "Apache-2.0"},
		URL:         "https://github.com/example/myapp",
		SourceURI:   []string{"https://github.com/example/myapp/archive/v2.1.0.tar.gz"},
		Depends:     []string{"openssl>=1.1.0", "zlib", "libcurl"},
		MakeDepends: []string{"gcc>=11", "cmake>=3.20", "git"},
		Maintainer:  "Test User <test@example.com>",
	}

	// Test artifact path
	artifactPath := filepath.Join(tmpDir, "myapp-2.1.0-3-x86_64.deb")

	// Generate both formats
	opts := sbom.Options{
		Formats: []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX},
	}

	generatedFiles, err := sbom.Generate(pkg, artifactPath, opts)
	require.NoError(t, err)
	require.Len(t, generatedFiles, 2)

	// Verify CycloneDX file
	cdxPath := artifactPath + ".cdx.json"
	assert.FileExists(t, cdxPath)

	cdxData, err := os.ReadFile(cdxPath)
	require.NoError(t, err)

	var cdxBOM sbom.CycloneDXBOM

	err = json.Unmarshal(cdxData, &cdxBOM)
	require.NoError(t, err)

	// Validate CycloneDX structure
	assert.Equal(t, "CycloneDX", cdxBOM.BOMFormat)
	assert.Equal(t, "1.5", cdxBOM.SpecVersion)
	assert.NotNil(t, cdxBOM.Metadata)
	assert.NotNil(t, cdxBOM.Metadata.Component)
	assert.Equal(t, "myapp", cdxBOM.Metadata.Component.Name)
	assert.Equal(t, "2.1.0", cdxBOM.Metadata.Component.Version)
	assert.Equal(t, "A sample application for testing SBOM generation",
		cdxBOM.Metadata.Component.Description)

	// Verify licenses
	assert.Len(t, cdxBOM.Metadata.Component.Licenses, 2)

	licenseNames := make(map[string]bool)
	for _, lic := range cdxBOM.Metadata.Component.Licenses {
		licenseNames[lic.License.Name] = true
	}

	assert.True(t, licenseNames["MIT"])
	assert.True(t, licenseNames["Apache-2.0"])

	// Verify external references
	assert.Len(t, cdxBOM.Metadata.Component.ExternalReferences, 1)
	assert.Equal(t, "distribution",
		cdxBOM.Metadata.Component.ExternalReferences[0].Type)
	assert.Equal(t, "https://github.com/example/myapp/archive/v2.1.0.tar.gz",
		cdxBOM.Metadata.Component.ExternalReferences[0].URL)

	// Verify components (dependencies)
	assert.NotEmpty(t, cdxBOM.Components)

	componentNames := make(map[string]bool)
	for _, comp := range cdxBOM.Components {
		componentNames[comp.Name] = true
	}

	assert.True(t, componentNames["openssl"])
	assert.True(t, componentNames["zlib"])
	assert.True(t, componentNames["libcurl"])
	assert.True(t, componentNames["gcc"])
	assert.True(t, componentNames["cmake"])
	assert.True(t, componentNames["git"])

	// Verify SPDX file
	spdxPath := artifactPath + ".spdx.json"
	assert.FileExists(t, spdxPath)

	spdxData, err := os.ReadFile(spdxPath)
	require.NoError(t, err)

	var spdxDoc sbom.SPDXDocument

	err = json.Unmarshal(spdxData, &spdxDoc)
	require.NoError(t, err)

	// Validate SPDX structure
	assert.Equal(t, "SPDX-2.3", spdxDoc.SPDXVersion)
	assert.Equal(t, "CC0-1.0", spdxDoc.DataLicense)
	assert.Equal(t, "SPDXRef-DOCUMENT", spdxDoc.SPDXID)
	assert.NotEmpty(t, spdxDoc.DocumentNamespace)
	assert.Contains(t, spdxDoc.DocumentNamespace, "myapp")
	assert.Contains(t, spdxDoc.DocumentNamespace, "2.1.0")

	// Verify creation info
	assert.NotNil(t, spdxDoc.CreationInfo)
	assert.NotEmpty(t, spdxDoc.CreationInfo.Created)
	assert.Contains(t, spdxDoc.CreationInfo.Creators, "Tool: yap")

	// Verify packages
	assert.NotEmpty(t, spdxDoc.Packages)
	mainPkg := spdxDoc.Packages[0]
	assert.Equal(t, "SPDXRef-Package", mainPkg.SPDXID)
	assert.Equal(t, "myapp", mainPkg.Name)
	assert.Equal(t, "2.1.0", mainPkg.Version)
	assert.False(t, mainPkg.FilesAnalyzed)

	// Verify licenses
	assert.Equal(t, "MIT OR Apache-2.0", mainPkg.LicenseConcluded)
	assert.Equal(t, "MIT OR Apache-2.0", mainPkg.LicenseDeclared)

	// Verify relationships
	assert.NotEmpty(t, spdxDoc.Relationships)

	hasDescribes := false
	dependsOnCount := 0

	for _, rel := range spdxDoc.Relationships {
		if rel.RelationshipType == "DESCRIBES" {
			hasDescribes = true
		}

		if rel.RelationshipType == "DEPENDS_ON" {
			dependsOnCount++
		}
	}

	assert.True(t, hasDescribes)
	assert.Greater(t, dependsOnCount, 0)
}

// TestSBOMWithMinimalPackage tests SBOM generation with minimal package info.
func TestSBOMWithMinimalPackage(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &pkgbuild.PKGBUILD{
		PkgName: "minimal",
		PkgVer:  "1.0",
		PkgRel:  "1",
	}

	artifactPath := filepath.Join(tmpDir, "minimal-1.0-1-x86_64.rpm")

	opts := sbom.Options{
		Formats: []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX},
	}

	generatedFiles, err := sbom.Generate(pkg, artifactPath, opts)
	require.NoError(t, err)
	require.Len(t, generatedFiles, 2)

	// Verify files exist
	assert.FileExists(t, artifactPath+".cdx.json")
	assert.FileExists(t, artifactPath+".spdx.json")

	// Verify CycloneDX is valid JSON
	cdxData, err := os.ReadFile(artifactPath + ".cdx.json")
	require.NoError(t, err)

	var cdxBOM sbom.CycloneDXBOM

	err = json.Unmarshal(cdxData, &cdxBOM)
	require.NoError(t, err)
	assert.Equal(t, "minimal", cdxBOM.Metadata.Component.Name)

	// Verify SPDX is valid JSON
	spdxData, err := os.ReadFile(artifactPath + ".spdx.json")
	require.NoError(t, err)

	var spdxDoc sbom.SPDXDocument

	err = json.Unmarshal(spdxData, &spdxDoc)
	require.NoError(t, err)
	assert.Equal(t, "SPDX-2.3", spdxDoc.SPDXVersion)
}

// TestSBOMFormatSelection tests different SBOM format selections.
func TestSBOMFormatSelection(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &pkgbuild.PKGBUILD{
		PkgName: "test",
		PkgVer:  "1.0",
		PkgRel:  "1",
	}

	tests := []struct {
		name      string
		formats   []sbom.Format
		wantFiles int
		wantCDX   bool
		wantSPDX  bool
	}{
		{
			name:      "CycloneDX only",
			formats:   []sbom.Format{sbom.FormatCycloneDX},
			wantFiles: 1,
			wantCDX:   true,
			wantSPDX:  false,
		},
		{
			name:      "SPDX only",
			formats:   []sbom.Format{sbom.FormatSPDX},
			wantFiles: 1,
			wantCDX:   false,
			wantSPDX:  true,
		},
		{
			name:      "Both formats",
			formats:   []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX},
			wantFiles: 2,
			wantCDX:   true,
			wantSPDX:  true,
		},
		{
			name:      "No formats",
			formats:   []sbom.Format{},
			wantFiles: 0,
			wantCDX:   false,
			wantSPDX:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifactPath := filepath.Join(tmpDir, tt.name+"-1.0-1-x86_64.deb")

			opts := sbom.Options{Formats: tt.formats}
			generatedFiles, err := sbom.Generate(pkg, artifactPath, opts)
			require.NoError(t, err)
			assert.Len(t, generatedFiles, tt.wantFiles)

			if tt.wantCDX {
				assert.FileExists(t, artifactPath+".cdx.json")
			} else {
				assert.NoFileExists(t, artifactPath+".cdx.json")
			}

			if tt.wantSPDX {
				assert.FileExists(t, artifactPath+".spdx.json")
			} else {
				assert.NoFileExists(t, artifactPath+".spdx.json")
			}
		})
	}
}
