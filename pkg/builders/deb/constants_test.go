package deb

import (
	"strings"
	"testing"
)

func TestSpecFileTemplate(t *testing.T) {
	if specFile == "" {
		t.Error("specFile template is empty")
	}

	requiredFields := []string{
		"Package:", "Version:", "Section:", "Priority:", "Description:",
	}

	for _, field := range requiredFields {
		if !strings.Contains(specFile, field) {
			t.Errorf("specFile template missing required field: %s", field)
		}
	}

	conditionalFields := []string{
		"{{.PkgName}}", "{{.PkgVer}}", "{{multiline .PkgDesc}}", "{{.ArchComputed}}",
	}

	for _, field := range conditionalFields {
		if !strings.Contains(specFile, field) {
			t.Errorf("specFile template missing template field: %s", field)
		}
	}
}

func TestRemoveHeaderScript(t *testing.T) {
	if removeHeader == "" {
		t.Error("removeHeader script is empty")
	}

	if !strings.Contains(removeHeader, "#!/bin/bash") {
		t.Error("removeHeader should contain shebang")
	}

	requiredCases := []string{"purge", "remove", "abort-install"}
	for _, caseType := range requiredCases {
		if !strings.Contains(removeHeader, caseType) {
			t.Errorf("removeHeader missing case: %s", caseType)
		}
	}
}

func TestCopyrightFileTemplate(t *testing.T) {
	if copyrightFile == "" {
		t.Error("copyrightFile template is empty")
	}

	requiredElements := []string{
		"Format:", "Upstream-Name:", "Files:", "{{.PkgName}}",
	}

	for _, element := range requiredElements {
		if !strings.Contains(copyrightFile, element) {
			t.Errorf("copyrightFile template missing element: %s", element)
		}
	}
}

func TestDEBConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"binary content", binaryContent, "2.0\n"},
		{"binary filename", binaryFilename, "debian-binary"},
		{"control filename", controlFilename, "control.tar.zst"},
		{"data filename", dataFilename, "data.tar.zst"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestDEBFilenames(t *testing.T) {
	filenames := []string{binaryFilename, controlFilename, dataFilename}

	for _, filename := range filenames {
		if filename == "" {
			t.Error("DEB filename constant is empty")
		}

		if strings.ContainsAny(filename, "/\\") {
			t.Errorf("DEB filename %q should not contain path separators", filename)
		}
	}
}

func TestDEBFileExtensions(t *testing.T) {
	if !strings.HasSuffix(controlFilename, ".tar.zst") {
		t.Errorf("controlFilename should end with .tar.zst, got %s", controlFilename)
	}

	if !strings.HasSuffix(dataFilename, ".tar.zst") {
		t.Errorf("dataFilename should end with .tar.zst, got %s", dataFilename)
	}
}
