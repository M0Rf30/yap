package debian

const specFile = `
{{- /* Mandatory fields */ -}}
Package: {{.PKGBUILD.PkgName}}
Version: {{ if .PKGBUILD.Epoch}}{{ .PKGBUILD.Epoch }}:{{ end }}{{.PKGBUILD.PkgVer}}
         {{- if .PKGBUILD.PreRelease}}~{{ .PKGBUILD.PreRelease }}{{- end }}
         {{- if .PKGBUILD.PkgRel}}-{{ .PKGBUILD.PkgRel }}{{- end }}
Section: {{.PKGBUILD.Section}}
Priority: {{.PKGBUILD.Priority}}
{{- with .PKGBUILD.Arch}}
Architecture: {{join .}}
{{- end }}
{{- /* Optional fields */ -}}
{{- if .PKGBUILD.Maintainer}}
Maintainer: {{.PKGBUILD.Maintainer}}
{{- end }}
Installed-Size: {{.InstalledSize}}
{{- with .PKGBUILD.Provides}}
Provides: {{join .}}
{{- end }}
{{- with .PKGBUILD.Depends}}
Depends: {{join .}}
{{- end }}
{{- with .PKGBUILD.Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- if .PKGBUILD.URL}}
Homepage: {{.PKGBUILD.URL}}
{{- end }}
{{- /* Mandatory fields */}}
Description: {{multiline .PKGBUILD.PkgDesc}}
`

const removeHeader = `#!/bin/bash
case $1 in
    purge|remove|abort-install) ;;
    *) exit;;
esac
`
