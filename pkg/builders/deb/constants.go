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

// maintainerScriptShebang is the interpreter line prepended to preinst and
// postinst maintainer scripts that do not already declare one. Debian
// Policy §6.1 requires every maintainer script to begin with a "#!"
// interpreter line, since dpkg execve()s these files directly rather than
// sourcing them. prerm and postrm already gain an interpreter line via
// removeHeader below, so they are left untouched.
const maintainerScriptShebang = "#!/bin/sh\n"

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
	binaryContent   = "2.0\n"
	binaryFilename  = "debian-binary"
	controlFilename = "control.tar.zst"
	dataFilename    = "data.tar.zst"
)
