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

func TestInstallScriptTemplate(t *testing.T) {
	if installScript == "" {
		t.Error("installScript template is empty")
	}

	if !strings.Contains(installScript, "#!/bin/sh") {
		t.Error("installScript should contain shebang")
	}

	requiredFunctions := []string{
		"pre_install()", "post_install()",
		"pre_upgrade()", "post_upgrade()",
		"pre_deinstall()", "post_deinstall()",
	}

	for _, function := range requiredFunctions {
		if !strings.Contains(installScript, function) {
			t.Errorf("installScript template missing function: %s", function)
		}
	}

	templateVars := []string{"{{.PreInst}}", "{{.PostInst}}", "{{.PreRm}}", "{{.PostRm}}"}

	for _, templateVar := range templateVars {
		if !strings.Contains(installScript, templateVar) {
			t.Errorf("installScript template missing template variable: %s", templateVar)
		}
	}
}

func TestTemplateConsistency(t *testing.T) {
	if dotPkginfo == "" {
		t.Error("dotPkginfo should not be empty")
	}

	if installScript == "" {
		t.Error("installScript should not be empty")
	}
}
