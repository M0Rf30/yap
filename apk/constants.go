package apk

const specFile = `
{{- /* Mandatory fields */ -}}
pkgname={{.Pack.PkgName}}
{{- with .Pack.Epoch}}
epoch={{.Pack.Epoch}}
{{- end }}
pkgver={{.Pack.PkgVer}}
pkgrel={{.Pack.PkgRel}}
pkgdesc="{{.Pack.PkgDesc}}"
arch="all"
{{- with .Pack.Depends}}
depends="{{join .}}"
{{- end }}
{{- with .Pack.Conflicts}}
conflicts=({{join .}})
{{- end }}
{{- if .Pack.URL}}
url="{{.Pack.URL}}"
{{- end }}
{{- if .Pack.Install}}
install={{.Pack.PkgName}}.install
{{- end }}
{{- if .Pack.License}}
license={{.Pack.License}}
{{- else }}
license="CUSTOM"
{{- end }}

options="!check !fhs"

package() {
  rsync -a -A {{.Pack.PackageDir}}/ ${pkgdir}
}
`
const postInstall = `
{{- if .Pack.PreInst}}
pre_install() {
  {{.Pack.PreInst}}"
}
{{- end }}

{{- if .Pack.PostInst}}
post_install() {
  {{.Pack.PostInst}}"
}
{{- end }}

{{- if .Pack.PreInst}}
pre_upgrade() {
  {{.Pack.PreInst}}"
}
{{- end }}

{{- if .Pack.PostInst}}
post_upgrade() {
  {{.Pack.PostInst}}"
}
{{- end }}

{{- if .Pack.PreRm}}
pre_remove() {
  {{.Pack.PreRm}}"
}
{{- end }}

{{- if .Pack.PostRm}}
post_remove() {
  {{.Pack.PostRm}}"
}
{{- end }}
`
