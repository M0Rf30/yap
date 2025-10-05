package dpkg

var buildEnvironmentDeps = []string{
	"autoconf",
	"binutils",
	"ca-certificates",
	"ccache",
	"g++",
	"gcc",
	"libc-dev",
	"libc6-dev",
	"libltdl7",
	"libtool",
	"make",
	"patch",
	"pkg-config",
	"tzdata",
}

var installArgs = []string{
	"--allow-downgrades",
	"--assume-yes",
	"install",
}

var DebArchs = map[string]string{
	"any":     "all",
	"x86_64":  "amd64",
	"i686":    "386",
	"aarch64": "arm64",
	"armv7h":  "arm7",
	"armv6h":  "arm6",
	"arm":     "arm5",
}

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
{{- /* Optional fields */ -}}
{{- if .Maintainer}}
Maintainer: {{.Maintainer}}
{{- end }}
Installed-Size: {{.InstalledSize}}
{{- with .Provides}}
Provides: {{join .}}
{{- end }}
{{- with .Depends}}
Depends: {{join .}}
{{- end }}
{{- with .Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .Replaces}}
Replaces: {{join .}}
{{- end }}
{{- if .URL}}
Homepage: {{.URL}}
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
	binaryContent   = "2.0\n"
	binaryFilename  = "debian-binary"
	controlFilename = "control.tar.zst"
	dataFilename    = "data.tar.zst"
)
