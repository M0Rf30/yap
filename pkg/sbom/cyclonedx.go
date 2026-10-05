package sbom

import (
	"fmt"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// componentTypeLibrary is the CycloneDX component type for library packages.
const componentTypeLibrary = "library"

// CycloneDXBOM represents a CycloneDX 1.5 Bill of Materials.
type CycloneDXBOM struct {
	BOMFormat          string                 `json:"bomFormat"`
	SpecVersion        string                 `json:"specVersion"`
	SerialNumber       string                 `json:"serialNumber"`
	Version            int                    `json:"version"`
	Metadata           *CycloneDXMetadata     `json:"metadata,omitempty"`
	Components         []*CycloneDXComponent  `json:"components,omitempty"`
	Dependencies       []*CycloneDXDependency `json:"dependencies,omitempty"`
	ExternalReferences []*CycloneDXExtRef     `json:"externalReferences,omitempty"`
}

// CycloneDXMetadata represents metadata in CycloneDX BOM.
type CycloneDXMetadata struct {
	Timestamp string              `json:"timestamp,omitempty"`
	Component *CycloneDXComponent `json:"component,omitempty"`
}

// CycloneDXComponent represents a component in CycloneDX BOM.
type CycloneDXComponent struct {
	Type               string              `json:"type"`
	BOMRef             string              `json:"bom-ref,omitempty"`
	Name               string              `json:"name"`
	Version            string              `json:"version,omitempty"`
	Description        string              `json:"description,omitempty"`
	Licenses           []*CycloneDXLicense `json:"licenses,omitempty"`
	Purl               string              `json:"purl,omitempty"`
	ExternalReferences []*CycloneDXExtRef  `json:"externalReferences,omitempty"`
	Hashes             []*CycloneDXHash    `json:"hashes,omitempty"`
}

// CycloneDXLicense represents a license in CycloneDX BOM.
type CycloneDXLicense struct {
	License *CycloneDXLicenseChoice `json:"license,omitempty"`
}

// CycloneDXLicenseChoice represents a license choice.
type CycloneDXLicenseChoice struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

// CycloneDXHash represents a hash in CycloneDX BOM.
type CycloneDXHash struct {
	Alg   string `json:"alg"`
	Value string `json:"content"`
}

// CycloneDXExtRef represents an external reference in CycloneDX BOM.
type CycloneDXExtRef struct {
	Type   string           `json:"type"`
	URL    string           `json:"url"`
	Hashes []*CycloneDXHash `json:"hashes,omitempty"`
}

// CycloneDXDependency represents a dependency in CycloneDX BOM.
type CycloneDXDependency struct {
	Ref     string   `json:"ref"`
	Depends []string `json:"depends,omitempty"`
}

// generateCycloneDX generates a CycloneDX 1.5 SBOM for the given package.
func generateCycloneDX(pkg *pkgbuild.PKGBUILD) *CycloneDXBOM {
	bom := &CycloneDXBOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  "1.5",
		SerialNumber: "urn:uuid:" + newSerialUUID(pkg),
		Version:      1,
	}

	// Create main component
	mainComponent := &CycloneDXComponent{
		Type:        componentTypeLibrary,
		BOMRef:      generatePurl(pkg),
		Name:        pkg.PkgName,
		Version:     pkg.PkgVer,
		Description: pkg.PkgDesc,
		Purl:        generatePurl(pkg),
	}

	// Add licenses
	for _, license := range pkg.License {
		mainComponent.Licenses = append(mainComponent.Licenses, &CycloneDXLicense{
			License: &CycloneDXLicenseChoice{
				Name: license,
			},
		})
	}

	// Add external references for source URLs, with checksums when they line
	// up one-to-one with the sources.
	checksumsAligned := len(pkg.HashSums) == len(pkg.SourceURI)

	for i, sourceURL := range pkg.SourceURI {
		ref := &CycloneDXExtRef{
			Type: "distribution",
			URL:  sourceURL,
		}

		if checksumsAligned {
			if h := hashFromSum(pkg.HashSums[i]); h != nil {
				ref.Hashes = []*CycloneDXHash{h}
			}
		}

		mainComponent.ExternalReferences = append(mainComponent.ExternalReferences, ref)
	}

	// Set metadata
	bom.Metadata = &CycloneDXMetadata{
		Timestamp: documentTime().Format("2006-01-02T15:04:05Z"),
		Component: mainComponent,
	}

	// Add runtime and make dependencies as components
	mainDep := &CycloneDXDependency{Ref: mainComponent.BOMRef}

	for _, dep := range collectDependencies(pkg) {
		purl := "pkg:generic/" + purlName(dep.name)
		bom.Components = append(bom.Components, &CycloneDXComponent{
			Type:   componentTypeLibrary,
			BOMRef: purl,
			Name:   dep.name,
			Purl:   purl,
		})

		if dep.runtime {
			mainDep.Depends = append(mainDep.Depends, purl)
		}
	}

	// Create dependencies relationships
	if len(mainDep.Depends) > 0 {
		bom.Dependencies = append(bom.Dependencies, mainDep)
	}

	return bom
}

// generatePurl generates a Package URL (purl) for the given package.
func generatePurl(pkg *pkgbuild.PKGBUILD) string {
	// Basic purl format: pkg:type/namespace/name@version
	// For generic packages: pkg:generic/name@version
	return fmt.Sprintf("pkg:generic/%s@%s", purlName(pkg.PkgName), purlName(pkg.PkgVer))
}

// extractDepName extracts the package name from a dependency string.
// Handles formats like "gcc", "gcc>=11.0", "glibc=2.38", "python3 >=3.9", etc.
func extractDepName(dep string) string {
	fields := strings.Fields(dep)
	if len(fields) == 0 {
		return ""
	}

	// Remove version constraints (any of <, >, =, !, ~ starts one).
	name := fields[0]
	if idx := strings.IndexAny(name, "<>=!~"); idx != -1 {
		name = name[:idx]
	}

	return strings.TrimSpace(name)
}
