package sbom

import (
	"regexp"
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
	assert.Equal(t, bom.Metadata.Component.BOMRef, bom.Dependencies[0].Ref)
	assert.Contains(t, bom.Dependencies[0].Depends, "pkg:generic/gcc")
	assert.Contains(t, bom.Dependencies[0].Depends, "pkg:generic/make")
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

func TestExtractDepNameSingleEquals(t *testing.T) {
	assert.Equal(t, "glibc", extractDepName("glibc=2.38"))
	assert.Equal(t, "glibc", extractDepName("glibc=2.38-1"))
	assert.Equal(t, "libfoo", extractDepName("libfoo~1.0"))
	assert.Equal(t, "libstdc++", extractDepName("libstdc++>=13"))
}

var (
	serialRe  = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	spdxIDRe  = regexp.MustCompile(`^SPDXRef-[A-Za-z0-9.-]+$`)
	sha256Sum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func TestCycloneDXSchemaCompliance(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:     "testpkg",
		PkgVer:      "1.0.0",
		SourceURI:   []string{"https://example.com/a.tar.gz", "https://example.com/b.tar.gz"},
		HashSums:    []string{sha256Sum, "SKIP"},
		Depends:     []string{"libstdc++", "glibc=2.38", "glibc>=2.30", "pkg:any"},
		MakeDepends: []string{"cmake"},
	}

	bom := generateCycloneDX(pkg)
	assert.Regexp(t, serialRe, bom.SerialNumber)

	// Serial numbers are unique per generation unless SOURCE_DATE_EPOCH is set.
	assert.NotEqual(t, bom.SerialNumber, generateCycloneDX(pkg).SerialNumber)

	refs := map[string]bool{bom.Metadata.Component.BOMRef: true}

	for _, c := range bom.Components {
		assert.NotEmpty(t, c.BOMRef)
		assert.False(t, refs[c.BOMRef], "duplicate bom-ref %q", c.BOMRef)
		refs[c.BOMRef] = true
	}

	// Duplicate Depends collapse to one component; '=' is stripped.
	assert.Len(t, bom.Components, 4)

	require.Len(t, bom.Dependencies, 1)
	assert.Equal(t, bom.Metadata.Component.BOMRef, bom.Dependencies[0].Ref)
	assert.Len(t, bom.Dependencies[0].Depends, 3)

	for _, ref := range bom.Dependencies[0].Depends {
		assert.True(t, refs[ref], "dangling dependency ref %q", ref)
	}

	// Checksums are attached to the matching distribution reference.
	extRefs := bom.Metadata.Component.ExternalReferences
	require.Len(t, extRefs, 2)
	require.Len(t, extRefs[0].Hashes, 1)
	assert.Equal(t, "SHA-256", extRefs[0].Hashes[0].Alg)
	assert.Equal(t, sha256Sum, extRefs[0].Hashes[0].Value)
	assert.Empty(t, extRefs[1].Hashes)
}

func TestCycloneDXReproducibleWithSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1609459200")

	pkg := &pkgbuild.PKGBUILD{PkgName: "testpkg", PkgVer: "1.0.0", PkgRel: "1"}
	a, b := generateCycloneDX(pkg), generateCycloneDX(pkg)

	assert.Regexp(t, serialRe, a.SerialNumber)
	assert.Equal(t, a.SerialNumber, b.SerialNumber)
	assert.Equal(t, "2021-01-01T00:00:00Z", a.Metadata.Timestamp)
	assert.Equal(t, "2021-01-01T00:00:00Z", generateSPDX(pkg).CreationInfo.Created)
}

func TestSPDXIDsAreValidAndUnique(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "testpkg",
		PkgVer:  "1.0.0",
		Depends: []string{
			"libstdc++", "lib_foo", "pkg:any", "libstdc++>=13", "libstdc--", "lib_foo=1",
		},
		MakeDepends: []string{"pkg:any", "cmake"},
	}

	doc := generateSPDX(pkg)

	ids := map[string]bool{}

	for _, p := range doc.Packages {
		assert.Regexp(t, spdxIDRe, p.SPDXID)
		assert.False(t, ids[p.SPDXID], "duplicate SPDXID %q", p.SPDXID)
		ids[p.SPDXID] = true
	}

	// main + libstdc++, lib_foo, pkg:any, libstdc-- (collides after sanitising), cmake
	assert.Len(t, doc.Packages, 6)

	seenRel := map[string]bool{}

	for _, r := range doc.Relationships {
		assert.True(t, ids[r.RelatedSpdxElement] || r.RelatedSpdxElement == "SPDXRef-Package")

		key := r.SpdxElementID + r.RelationshipType + r.RelatedSpdxElement
		assert.False(t, seenRel[key], "duplicate relationship %s", key)
		seenRel[key] = true
	}
}
