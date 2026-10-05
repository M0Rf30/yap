// Package deb provides Debian package building functionality and constants.
package deb

// Template constants - these are DEB-specific templates that should remain here
const specFile = `
{{- /* Mandatory fields */ -}}
Package: {{.PkgName}}
Version: {{ if .Epoch}}{{ .Epoch }}:{{ end }}{{.PkgVer}}
         {{- if .PkgRel}}-{{ .PkgRel }}{{- end }}
Section: {{.Section}}
Priority: {{.Priority}}
{{- if .ArchComputed}}
Architecture: {{.ArchComputed}}
{{- end }}
{{- if .MultiArch}}
Multi-Arch: {{.MultiArch}}
{{- else if eq .ArchComputed "all"}}
Multi-Arch: foreign
{{- end }}
{{- /* Optional fields */ -}}
{{- if .Maintainer}}
Maintainer: {{.Maintainer}}
{{- end }}
{{- if .SourcePkg}}
Source: {{.SourcePkg}}
{{- end }}
Installed-Size: {{.InstalledSize}}
{{- with .Provides}}
Provides: {{join .}}
{{- end }}
{{- with .PreDepends}}
Pre-Depends: {{join .}}
{{- end }}
{{- with .Depends}}
Depends: {{join .}}
{{- end }}
{{- with .Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .Breaks}}
Breaks: {{join .}}
{{- end }}
{{- with .Replaces}}
Replaces: {{join .}}
{{- end }}
{{- with .OptDepends}}
Recommends: {{join .}}
{{- end }}
{{- with .Suggests}}
Suggests: {{join .}}
{{- end }}
{{- with .Enhances}}
Enhances: {{join .}}
{{- end }}
{{- with .BuiltUsing}}
Built-Using: {{join .}}
{{- end }}
{{- if .URL}}
Homepage: {{.URL}}
{{- end }}
{{- if .Bugs}}
Bugs: {{.Bugs}}
{{- end }}
{{- /* Mandatory fields */}}
Description: {{multiline .PkgDesc}}
`

const removeHeader = `#!/bin/bash
case $1 in
    purge|remove|abort-install) ;;
    *) exit;;
esac
`

const copyrightFile = `Format: http://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: {{.PkgName}}
Upstream-Contact: {{.Maintainer}}
{{- if .URL}}
Source: {{.URL}}
{{- end }}
Files: *
{{- if .Copyright}}
Copyright: {{ range .Copyright}}{{ . }}
           {{ end }}{{- end }}
{{- if .License}}
{{- range .License}}
License: {{ . }}{{- end }}
{{- end }}
`

const (
	binaryContent  = "2.0\n"
	binaryFilename = "debian-binary"
	// controlBasename and dataBasename are the ar member names without the
	// compression suffix; dpkg picks the decompressor from that suffix.
	controlBasename = "control.tar"
	dataBasename    = "data.tar"
)

// compressionSuffix returns the ar member suffix dpkg expects for the given
// compression algorithm ("zstd" -> ".zst", "gzip" -> ".gz", "xz" -> ".xz").
// Unknown or empty values map to ".zst", the default algorithm.
func compressionSuffix(compression string) string {
	switch compression {
	case "gzip":
		return ".gz"
	case "xz":
		return ".xz"
	default:
		return ".zst"
	}
}

// memberNames returns the control and data ar member names for the
// compression algorithm, e.g. control.tar.gz / data.tar.gz.
func memberNames(compression string) (control, data string) {
	suffix := compressionSuffix(compression)

	return controlBasename + suffix, dataBasename + suffix
}
