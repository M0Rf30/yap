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

func TestGenerateToFiles(t *testing.T) {
	tmpDir := t.TempDir()
	artifactPath := filepath.Join(tmpDir, "testpkg-1.0.0-1-x86_64.deb")

	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		PkgRel:      "1",
		PkgDesc:     "Test package",
		License:     []string{"MIT"},
		SourceURI:   []string{"https://example.com/testpkg-1.0.0.tar.gz"},
		Depends:     []string{"gcc"},
		MakeDepends: []string{"git"},
	}

	opts := sbom.Options{
		Formats: []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX},
	}

	generatedFiles, err := sbom.Generate(pkg, artifactPath, opts)
	require.NoError(t, err)
	assert.Len(t, generatedFiles, 2)

	// Verify CycloneDX file
	cdxPath := artifactPath + ".cdx.json"
	assert.FileExists(t, cdxPath)
	cdxData, err := os.ReadFile(cdxPath)
	require.NoError(t, err)

	var cdxBOM sbom.CycloneDXBOM

	err = json.Unmarshal(cdxData, &cdxBOM)
	require.NoError(t, err)
	assert.Equal(t, "CycloneDX", cdxBOM.BOMFormat)
	assert.Equal(t, "1.5", cdxBOM.SpecVersion)

	// Verify SPDX file
	spdxPath := artifactPath + ".spdx.json"
	assert.FileExists(t, spdxPath)
	spdxData, err := os.ReadFile(spdxPath)
	require.NoError(t, err)

	var spdxDoc sbom.SPDXDocument

	err = json.Unmarshal(spdxData, &spdxDoc)
	require.NoError(t, err)
	assert.Equal(t, "SPDX-2.3", spdxDoc.SPDXVersion)
	assert.Equal(t, "CC0-1.0", spdxDoc.DataLicense)
}

func TestGenerateEmptyFormats(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "testpkg",
		PkgVer:  "1.0.0",
	}

	opts := sbom.Options{
		Formats: []sbom.Format{},
	}

	generatedFiles, err := sbom.Generate(pkg, "/tmp/artifact", opts)
	require.NoError(t, err)
	assert.Empty(t, generatedFiles)
}

func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    sbom.Options
		wantErr bool
	}{
		{
			name:    "valid cyclonedx",
			opts:    sbom.Options{Formats: []sbom.Format{sbom.FormatCycloneDX}},
			wantErr: false,
		},
		{
			name:    "valid spdx",
			opts:    sbom.Options{Formats: []sbom.Format{sbom.FormatSPDX}},
			wantErr: false,
		},
		{
			name:    "valid both",
			opts:    sbom.Options{Formats: []sbom.Format{sbom.FormatCycloneDX, sbom.FormatSPDX}},
			wantErr: false,
		},
		{
			name:    "invalid format",
			opts:    sbom.Options{Formats: []sbom.Format{"invalid"}},
			wantErr: true,
		},
		{
			name:    "empty formats",
			opts:    sbom.Options{Formats: []sbom.Format{}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sbom.ValidateOptions(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
