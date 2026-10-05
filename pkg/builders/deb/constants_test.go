// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

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
		{"control basename", controlBasename, "control.tar"},
		{"data basename", dataBasename, "data.tar"},
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
	for _, compression := range []string{"", "zstd", "gzip", "xz"} {
		control, data := memberNames(compression)
		filenames := []string{binaryFilename, control, data}

		for _, filename := range filenames {
			if filename == "" {
				t.Error("DEB filename is empty")
			}

			if strings.ContainsAny(filename, "/\\") {
				t.Errorf("DEB filename %q should not contain path separators", filename)
			}
		}
	}
}

func TestDEBMemberNames(t *testing.T) {
	tests := []struct {
		compression string
		control     string
		data        string
	}{
		{"", "control.tar.zst", "data.tar.zst"},
		{"zstd", "control.tar.zst", "data.tar.zst"},
		{"gzip", "control.tar.gz", "data.tar.gz"},
		{"xz", "control.tar.xz", "data.tar.xz"},
	}

	for _, tt := range tests {
		control, data := memberNames(tt.compression)
		if control != tt.control || data != tt.data {
			t.Errorf("memberNames(%q) = %q, %q; want %q, %q",
				tt.compression, control, data, tt.control, tt.data)
		}
	}
}
