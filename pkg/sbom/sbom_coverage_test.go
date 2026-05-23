package sbom //nolint:testpackage // white-box tests require access to unexported functions

// sbom_coverage_test.go — internal white-box tests targeting uncovered branches
// in sbom.go (Generate function).
//
// The only remaining uncovered branch in Generate is the json.MarshalIndent
// error path (line ~64). json.Marshal cannot fail on well-formed structs, so
// that branch is effectively dead code and cannot be exercised without
// reflection tricks. All other branches are already covered by existing tests.
//
// This file adds tests for edge cases in cyclonedx.go and spdx.go that are
// not yet fully covered.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestGenerateCycloneDXNoDeps exercises the path where both Depends and
// MakeDepends are empty — no components or dependency entries should be added.
func TestGenerateCycloneDXNoDeps(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "nodeps",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		PkgDesc: "Package with no dependencies",
		License: []string{"MIT"},
	}

	bom := generateCycloneDX(pkg)
	require.NotNil(t, bom)

	assert.Empty(t, bom.Components)
	// Dependencies slice may be empty or contain only the root with no deps
	if len(bom.Dependencies) > 0 {
		assert.Empty(t, bom.Dependencies[0].Depends)
	}
}

// TestGenerateCycloneDXNoSourceURI exercises the path where SourceURI is empty
// — no external references should be added to the metadata component.
func TestGenerateCycloneDXNoSourceURI(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "nosource",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		License: []string{"MIT"},
		// No SourceURI
	}

	bom := generateCycloneDX(pkg)
	require.NotNil(t, bom)
	require.NotNil(t, bom.Metadata)
	require.NotNil(t, bom.Metadata.Component)

	assert.Empty(t, bom.Metadata.Component.ExternalReferences)
}

// TestGenerateCycloneDXNoLicense exercises the path where License is empty
// — the licenses slice on the metadata component should be empty.
func TestGenerateCycloneDXNoLicense(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "nolic",
		PkgVer:  "1.0.0",
		License: []string{},
	}

	bom := generateCycloneDX(pkg)
	require.NotNil(t, bom)
	require.NotNil(t, bom.Metadata)
	require.NotNil(t, bom.Metadata.Component)

	assert.Empty(t, bom.Metadata.Component.Licenses)
}

// TestGenerateSPDXWithURL exercises the homepage URL path in generateSPDX
// when SourceURI is empty but URL is set.
func TestGenerateSPDXWithURL(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "urlonly",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		URL:     "https://example.com/urlonly",
		License: []string{"GPL-2.0"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)
	require.NotEmpty(t, doc.Packages)

	mainPkg := doc.Packages[0]
	assert.Equal(t, "https://example.com/urlonly", mainPkg.DownloadLocation)
}

// TestGenerateSPDXNoDeps exercises the path where both Depends and MakeDepends
// are empty — only the main package should appear, with a DESCRIBES relationship.
func TestGenerateSPDXNoDeps(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "nodeps",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		License: []string{"MIT"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)

	// Only the main package
	assert.Len(t, doc.Packages, 1)

	// Should have exactly one DESCRIBES relationship
	assert.Len(t, doc.Relationships, 1)
	assert.Equal(t, "DESCRIBES", doc.Relationships[0].RelationshipType)
}

// TestGenerateSPDXMultipleLicenses exercises the "MIT OR Apache-2.0" join path.
func TestGenerateSPDXMultipleLicenses(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "multilicense",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		License: []string{"MIT", "Apache-2.0", "GPL-2.0"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)
	require.NotEmpty(t, doc.Packages)

	mainPkg := doc.Packages[0]
	assert.Equal(t, "MIT OR Apache-2.0 OR GPL-2.0", mainPkg.LicenseConcluded)
	assert.Equal(t, "MIT OR Apache-2.0 OR GPL-2.0", mainPkg.LicenseDeclared)
}

// TestGenerateSPDXEmptyDepName exercises the path where a dep string like
// ">=1.0" produces an empty name and should be skipped.
func TestGenerateSPDXEmptyDepName(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "test",
		PkgVer:      "1.0.0",
		PkgRel:      "1",
		Depends:     []string{">=1.0"},
		MakeDepends: []string{">=2.0"},
	}

	doc := generateSPDX(pkg)
	require.NotNil(t, doc)

	// Only the main package — empty-name deps are skipped
	assert.Len(t, doc.Packages, 1)
}

// TestGetDownloadLocationEmptySourceURI exercises the URL fallback when
// SourceURI slice is non-nil but empty.
func TestGetDownloadLocationEmptySourceURI(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		SourceURI: []string{},
		URL:       "https://fallback.example.com",
	}

	loc := getDownloadLocation(pkg)
	assert.Equal(t, "https://fallback.example.com", loc)
}

// TestGeneratePurlWithVersion exercises generatePurl with a specific version.
func TestGeneratePurlWithVersion(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "mypkg",
		PkgVer:  "2.3.4",
	}

	purl := generatePurl(pkg)
	assert.Equal(t, "pkg:generic/mypkg@2.3.4", purl)
}

// TestGenerateDocumentNamespaceFormat verifies the namespace URL format.
func TestGenerateDocumentNamespaceFormat(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "mypkg",
		PkgVer:  "3.0.0",
		PkgRel:  "2",
	}

	ns := generateDocumentNamespace(pkg)
	assert.Equal(t, "https://yap.build/sbom/mypkg-3.0.0-2", ns)
}
