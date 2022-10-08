package debian

const specFile = `
{{- /* Mandatory fields */ -}}
Package: {{.Pack.PkgName}}
Version: {{ if .Pack.Epoch}}{{ .Pack.Epoch }}:{{ end }}{{.Pack.PkgVer}}
         {{- if .Pack.PreRelease}}~{{ .Pack.PreRelease }}{{- end }}
         {{- if .Pack.PkgRel}}-{{ .Pack.PkgRel }}{{- end }}
Section: {{.Pack.Section}}
Priority: {{.Pack.Priority}}
{{- with .Pack.Arch}}
Architecture: {{join .}}
{{- end }}
{{- /* Optional fields */ -}}
{{- if .Pack.Maintainer}}
Maintainer: {{.Pack.Maintainer}}
{{- end }}
Installed-Size: {{.InstalledSize}}
{{- with .Pack.Provides}}
Provides: {{join .}}
{{- end }}
{{- with .Pack.Depends}}
Depends: {{join .}}
{{- end }}
{{- with .Pack.Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- if .Pack.URL}}
Homepage: {{.Pack.URL}}
{{- end }}
{{- /* Mandatory fields */}}
Description: {{multiline .Pack.PkgDesc}}
`

const removeHeader = `#!/bin/bash
case $1 in
    purge|remove|abort-install) ;;
    *) exit;;
esac
`
