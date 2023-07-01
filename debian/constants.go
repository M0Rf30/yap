package debian

var buildEnvironmentDeps = []string{
	"build-essential",
	"reprepro",
	"tzdata",
	"ca-certificates",
}

var ArchToDebian = map[string]string{
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
         {{- if .PreRelease}}~{{ .PreRelease }}{{- end }}
         {{- if .PkgRel}}-{{ .PkgRel }}{{- end }}
Section: {{.Section}}
Priority: {{.Priority}}
{{- with .Arch}}
Architecture: {{join .}}
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
