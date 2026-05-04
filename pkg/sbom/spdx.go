package sbom

import (
	"fmt"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// SPDXDocument represents an SPDX 2.3 document.
type SPDXDocument struct {
	SPDXVersion       string              `json:"spdxVersion"`
	DataLicense       string              `json:"dataLicense"`
	SPDXID            string              `json:"SPDXID"`
	Name              string              `json:"name"`
	DocumentNamespace string              `json:"documentNamespace"`
	CreationInfo      *SPDXCreationInfo   `json:"creationInfo"`
	Packages          []*SPDXPackage      `json:"packages,omitempty"`
	Relationships     []*SPDXRelationship `json:"relationships,omitempty"`
}

// SPDXCreationInfo represents creation information in SPDX document.
type SPDXCreationInfo struct {
	Created            string   `json:"created"`
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

// SPDXPackage represents a package in SPDX document.
type SPDXPackage struct {
	SPDXID             string             `json:"SPDXID"`
	Name               string             `json:"name"`
	Version            string             `json:"versionInfo,omitempty"`
	Description        string             `json:"description,omitempty"`
	DownloadLocation   string             `json:"downloadLocation"`
	FilesAnalyzed      bool               `json:"filesAnalyzed"`
	LicenseConcluded   string             `json:"licenseConcluded,omitempty"`
	LicenseDeclared    string             `json:"licenseDeclared,omitempty"`
	CopyrightText      string             `json:"copyrightText,omitempty"`
	ExternalReferences []*SPDXExternalRef `json:"externalReferences,omitempty"`
}

// SPDXExternalRef represents an external reference in SPDX package.
type SPDXExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

// SPDXRelationship represents a relationship in SPDX document.
type SPDXRelationship struct {
	SpdxElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSpdxElement string `json:"relatedSpdxElement"`
}

const (
	// spdxRefPackage is the SPDX identifier for the main package.
	spdxRefPackage = "SPDXRef-Package"
	// noAssertion is the SPDX value used when information is not asserted.
	noAssertion = "NOASSERTION"
)

// generateSPDX generates an SPDX 2.3 SBOM for the given package.
func generateSPDX(pkg *pkgbuild.PKGBUILD) *SPDXDocument {
	doc := &SPDXDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              fmt.Sprintf("YAP Package: %s", pkg.PkgName),
		DocumentNamespace: generateDocumentNamespace(pkg),
		CreationInfo: &SPDXCreationInfo{
			Created:  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			Creators: []string{"Tool: yap"},
		},
	}

	// Create main package
	mainPkg := &SPDXPackage{
		SPDXID:           spdxRefPackage,
		Name:             pkg.PkgName,
		Version:          pkg.PkgVer,
		Description:      pkg.PkgDesc,
		DownloadLocation: getDownloadLocation(pkg),
		FilesAnalyzed:    false,
		CopyrightText:    noAssertion,
	}

	// Add licenses
	if len(pkg.License) > 0 {
		mainPkg.LicenseConcluded = strings.Join(pkg.License, " OR ")
		mainPkg.LicenseDeclared = strings.Join(pkg.License, " OR ")
	} else {
		mainPkg.LicenseConcluded = noAssertion
		mainPkg.LicenseDeclared = noAssertion
	}

	// Add external references for source URLs
	for _, sourceURL := range pkg.SourceURI {
		mainPkg.ExternalReferences = append(
			mainPkg.ExternalReferences,
			&SPDXExternalRef{
				ReferenceCategory: "PACKAGE-MANAGER",
				ReferenceType:     "source-url",
				ReferenceLocator:  sourceURL,
			},
		)
	}

	doc.Packages = append(doc.Packages, mainPkg)

	// Add runtime dependencies as packages
	depPackages := make(map[string]*SPDXPackage)

	for _, dep := range pkg.Depends {
		depName := extractDepName(dep)
		if depName == "" {
			continue
		}

		depPkg := &SPDXPackage{
			SPDXID:           fmt.Sprintf("SPDXRef-Dependency-%s", depName),
			Name:             depName,
			DownloadLocation: noAssertion,
			FilesAnalyzed:    false,
			CopyrightText:    noAssertion,
			LicenseConcluded: noAssertion,
			LicenseDeclared:  noAssertion,
		}
		depPackages[depName] = depPkg
		doc.Packages = append(doc.Packages, depPkg)
	}

	// Add make dependencies as packages
	for _, dep := range pkg.MakeDepends {
		depName := extractDepName(dep)
		if depName == "" || depPackages[depName] != nil {
			continue
		}

		depPkg := &SPDXPackage{
			SPDXID:           fmt.Sprintf("SPDXRef-Dependency-%s", depName),
			Name:             depName,
			DownloadLocation: noAssertion,
			FilesAnalyzed:    false,
			CopyrightText:    noAssertion,
			LicenseConcluded: noAssertion,
			LicenseDeclared:  noAssertion,
		}
		depPackages[depName] = depPkg
		doc.Packages = append(doc.Packages, depPkg)
	}

	// Create relationships
	// Main relationship: document describes package
	doc.Relationships = append(doc.Relationships, &SPDXRelationship{
		SpdxElementID:      "SPDXRef-DOCUMENT",
		RelationshipType:   "DESCRIBES",
		RelatedSpdxElement: spdxRefPackage,
	})

	// Dependency relationships
	for _, dep := range pkg.Depends {
		depName := extractDepName(dep)
		if depName != "" {
			doc.Relationships = append(doc.Relationships, &SPDXRelationship{
				SpdxElementID:      spdxRefPackage,
				RelationshipType:   "DEPENDS_ON",
				RelatedSpdxElement: fmt.Sprintf("SPDXRef-Dependency-%s", depName),
			})
		}
	}

	return doc
}

// generateDocumentNamespace generates a unique document namespace for SPDX.
func generateDocumentNamespace(pkg *pkgbuild.PKGBUILD) string {
	// Format: https://yap.build/sbom/<pkg>-<ver>-<rel>
	return fmt.Sprintf("https://yap.build/sbom/%s-%s-%s",
		pkg.PkgName, pkg.PkgVer, pkg.PkgRel)
}

// getDownloadLocation returns the download location for the package.
// Uses the first source URL if available, otherwise returns NOASSERTION.
func getDownloadLocation(pkg *pkgbuild.PKGBUILD) string {
	if len(pkg.SourceURI) > 0 {
		return pkg.SourceURI[0]
	}

	if pkg.URL != "" {
		return pkg.URL
	}

	return noAssertion
}
