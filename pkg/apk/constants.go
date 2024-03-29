package apk

var buildEnvironmentDeps = []string{
	"alpine-sdk",
}

const postInstall = `
{{- if .PreInst}}
pre_install() {
{{.PreInst}}
}
{{- end }}

{{- if .PostInst}}
post_install() {
{{.PostInst}}
}
{{- end }}

{{- if .PreInst}}
pre_upgrade() {
{{.PreInst}}
}
{{- end }}

{{- if .PostInst}}
post_upgrade() {
{{.PostInst}}
}
{{- end }}

{{- if .PreRm}}
pre_remove() {
{{.PreRm}}
}
{{- end }}

{{- if .PostRm}}
post_remove() {
{{.PostRm}}
}
{{- end }}
`

const specFile = `
{{- /* Mandatory fields */ -}}
pkgname={{.PkgName}}
{{- if .Epoch}}
epoch={{.Epoch}}
{{- end }}
pkgver={{.PkgVer}}
pkgrel={{.PkgRel}}
pkgdesc="{{.PkgDesc}}"
arch="all"
{{- if .Depends}}
depends="
{{ range .Depends }}{{ . }} 
{{ end }}"
{{- end }}
{{- if .URL}}
url="{{.URL}}"
{{- end }}
{{- if .Install}}
install={{.PkgName}}.install
{{- end }}
{{- if .License}}
license={{.License}}
{{- else }}
license="CUSTOM"
{{- end }}

options="!check !fhs"

package() {
{{.Package}}
}
`
