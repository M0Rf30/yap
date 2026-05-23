package sbom

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestExtractDepName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gcc", "gcc"},
		{"gcc >=11", "gcc"},
		{"gcc <=11", "gcc"},
		{"gcc =11", "gcc"},
		{"gcc >11", "gcc"},
		{"gcc <11", "gcc"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractDepName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGeneratePurl(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "testpkg",
		PkgVer:  "1.0.0",
	}

	purl := generatePurl(pkg)
	assert.Equal(t, "pkg:generic/testpkg@1.0.0", purl)
}

func TestGenerateDocumentNamespace(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "testpkg",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	ns := generateDocumentNamespace(pkg)
	assert.Equal(t, "https://yap.build/sbom/testpkg-1.0.0-1", ns)
}

func TestGenerateCycloneDX(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		PkgRel:      "1",
		PkgDesc:     "Test package description",
		License:     []string{"MIT", "Apache-2.0"},
		SourceURI:   []string{"https://example.com/testpkg-1.0.0.tar.gz"},
		Depends:     []string{"gcc", "make>=4.0"},
		MakeDepends: []string{"git", "cmake"},
	}

	bom := generateCycloneDX(pkg)
	require.NotNil(t, bom)

	// Verify BOM structure
	assert.Equal(t, "CycloneDX", bom.BOMFormat)
	assert.Equal(t, "1.5", bom.SpecVersion)
	assert.NotEmpty(t, bom.SerialNumber)

	// Verify metadata component
	require.NotNil(t, bom.Metadata)
	require.NotNil(t, bom.Metadata.Component)
	assert.Equal(t, "testpkg", bom.Metadata.Component.Name)
	assert.Equal(t, "1.0.0", bom.Metadata.Component.Version)
	assert.Equal(t, "Test package description", bom.Metadata.Component.Description)

	// Verify licenses
	assert.Len(t, bom.Metadata.Component.Licenses, 2)
	assert.Equal(t, "MIT", bom.Metadata.Component.Licenses[0].License.Name)
	assert.Equal(t, "Apache-2.0", bom.Metadata.Component.Licenses[1].License.Name)

	// Verify external references
	assert.Len(t, bom.Metadata.Component.ExternalReferences, 1)
	assert.Equal(t, "distribution", bom.Metadata.Component.ExternalReferences[0].Type)
	assert.Equal(t, "https://example.com/testpkg-1.0.0.tar.gz",
		bom.Metadata.Component.ExternalReferences[0].URL)

	// Verify components (dependencies)
	assert.NotEmpty(t, bom.Components)

	componentNames := make(map[string]bool)
	for _, comp := range bom.Components {
		componentNames[comp.Name] = true
	}

	assert.True(t, componentNames["gcc"])
	assert.True(t, componentNames["make"])
	assert.True(t, componentNames["git"])
	assert.True(t, componentNames["cmake"])

	// Verify dependencies relationships
	assert.NotEmpty(t, bom.Dependencies)
	assert.Equal(t, "testpkg", bom.Dependencies[0].Ref)
	assert.Contains(t, bom.Dependencies[0].Depends, "gcc")
	assert.Contains(t, bom.Dependencies[0].Depends, "make")
}

func TestGenerateSPDX(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		PkgRel:      "1",
		PkgDesc:     "Test package description",
		License:     []string{"MIT"},
		SourceURI:   []string{"https://example.com/testpkg-1.0.0.tar.gz"},
		URL:         "https://example.com",
		Depends:     []string{"gcc"},
		MakeDepends: []string{"git"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)

	// Verify document structure
	assert.Equal(t, "SPDX-2.3", doc.SPDXVersion)
	assert.Equal(t, "CC0-1.0", doc.DataLicense)
	assert.Equal(t, "SPDXRef-DOCUMENT", doc.SPDXID)
	assert.NotEmpty(t, doc.DocumentNamespace)

	// Verify creation info
	require.NotNil(t, doc.CreationInfo)
	assert.NotEmpty(t, doc.CreationInfo.Created)
	assert.Contains(t, doc.CreationInfo.Creators, "Tool: yap")

	// Verify packages
	assert.NotEmpty(t, doc.Packages)
	mainPkg := doc.Packages[0]
	assert.Equal(t, "SPDXRef-Package", mainPkg.SPDXID)
	assert.Equal(t, "testpkg", mainPkg.Name)
	assert.Equal(t, "1.0.0", mainPkg.Version)
	assert.Equal(t, "Test package description", mainPkg.Description)
	assert.False(t, mainPkg.FilesAnalyzed)

	// Verify licenses
	assert.Equal(t, "MIT", mainPkg.LicenseConcluded)
	assert.Equal(t, "MIT", mainPkg.LicenseDeclared)
}

func TestGetDownloadLocation(t *testing.T) {
	t.Run("has SourceURI", func(t *testing.T) {
		pkg := &pkgbuild.PKGBUILD{
			SourceURI: []string{"https://example.com/pkg-1.0.tar.gz"},
			URL:       "https://example.com",
		}
		assert.Equal(t, "https://example.com/pkg-1.0.tar.gz", getDownloadLocation(pkg))
	})

	t.Run("no SourceURI but has URL", func(t *testing.T) {
		pkg := &pkgbuild.PKGBUILD{
			URL: "https://example.com",
		}
		assert.Equal(t, "https://example.com", getDownloadLocation(pkg))
	})

	t.Run("neither SourceURI nor URL", func(t *testing.T) {
		pkg := &pkgbuild.PKGBUILD{}
		assert.Equal(t, "NOASSERTION", getDownloadLocation(pkg))
	})
}

func TestGenerateCycloneDXEmptyDepName(t *testing.T) {
	// A dep string like ">=1.0" has no name before the operator — should be skipped.
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		Depends:     []string{">=1.0"},
		MakeDepends: []string{">=2.0"},
	}

	bom := generateCycloneDX(pkg)
	require.NotNil(t, bom)

	// No components should have been added for the empty-name deps.
	assert.Empty(t, bom.Components)
	// The dependency entry exists (pkg.Depends is non-empty) but has no resolved dep names.
	if len(bom.Dependencies) > 0 {
		assert.Empty(t, bom.Dependencies[0].Depends)
	}
}

func TestGenerateSPDXNoLicense(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "testpkg",
		PkgVer:  "1.0.0",
		License: []string{}, // empty — should produce NOASSERTION
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)
	require.NotEmpty(t, doc.Packages)

	mainPkg := doc.Packages[0]
	assert.Equal(t, "NOASSERTION", mainPkg.LicenseConcluded)
	assert.Equal(t, "NOASSERTION", mainPkg.LicenseDeclared)
}

func TestGenerateSPDXMakeDepsDeduplicated(t *testing.T) {
	// "gcc" appears in both Depends and MakeDepends — should only be added once.
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		Depends:     []string{"gcc"},
		MakeDepends: []string{"gcc", "cmake"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)

	// Count packages named "gcc".
	gccCount := 0

	for _, p := range doc.Packages {
		if p.Name == "gcc" {
			gccCount++
		}
	}

	assert.Equal(t, 1, gccCount, "gcc should appear exactly once even though it is in both Depends and MakeDepends")

	// cmake should still be present.
	cmakeFound := false

	for _, p := range doc.Packages {
		if p.Name == "cmake" {
			cmakeFound = true
		}
	}

	assert.True(t, cmakeFound)
}
