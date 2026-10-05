package apk

import (
	"strings"
	"testing"
)

func TestDotPkginfoTemplate(t *testing.T) {
	if dotPkginfo == "" {
		t.Error("dotPkginfo template is empty")
	}

	requiredFields := []string{
		"pkgname =", "pkgver =", "pkgdesc =", "url =", "builddate =",
		"size =", "arch =",
	}

	for _, field := range requiredFields {
		if !strings.Contains(dotPkginfo, field) {
			t.Errorf("dotPkginfo template missing required field: %s", field)
		}
	}

	templateVars := []string{
		"{{.PkgName}}", "{{.PkgVer}}", "{{.PkgDesc}}", "{{.URL}}",
		"{{.BuildDate}}", "{{.InstalledSize}}", "{{.ArchComputed}}",
	}

	for _, templateVar := range templateVars {
		if !strings.Contains(dotPkginfo, templateVar) {
			t.Errorf("dotPkginfo template missing template variable: %s", templateVar)
		}
	}
}

func TestTemplateConsistency(t *testing.T) {
	if dotPkginfo == "" {
		t.Error("dotPkginfo should not be empty")
	}
}
