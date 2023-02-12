package apk

const specFile = `
{{- /* Mandatory fields */ -}}
pkgname={{.PKGBUILD.PkgName}}
{{- with .PKGBUILD.Epoch}}
epoch={{.PKGBUILD.Epoch}}
{{- end }}
pkgver={{.PKGBUILD.PkgVer}}
pkgrel={{.PKGBUILD.PkgRel}}
pkgdesc="{{.PKGBUILD.PkgDesc}}"
arch="all"
{{- with .PKGBUILD.Depends}}
depends="{{join .}}"
{{- end }}
{{- with .PKGBUILD.Conflicts}}
conflicts=({{join .}})
{{- end }}
{{- if .PKGBUILD.URL}}
url="{{.PKGBUILD.URL}}"
{{- end }}
{{- if .PKGBUILD.Install}}
install={{.PKGBUILD.PkgName}}.install
{{- end }}
{{- if .PKGBUILD.License}}
license={{.PKGBUILD.License}}
{{- else }}
license="CUSTOM"
{{- end }}

options="!check !fhs"

package() {
  rsync -a -A {{.PKGBUILD.PackageDir}}/ ${pkgdir}
}
`
const postInstall = `
{{- if .PKGBUILD.PreInst}}
pre_install() {
  {{.PKGBUILD.PreInst}}"
}
{{- end }}

{{- if .PKGBUILD.PostInst}}
post_install() {
  {{.PKGBUILD.PostInst}}"
}
{{- end }}

{{- if .PKGBUILD.PreInst}}
pre_upgrade() {
  {{.PKGBUILD.PreInst}}"
}
{{- end }}

{{- if .PKGBUILD.PostInst}}
post_upgrade() {
  {{.PKGBUILD.PostInst}}"
}
{{- end }}

{{- if .PKGBUILD.PreRm}}
pre_remove() {
  {{.PKGBUILD.PreRm}}"
}
{{- end }}

{{- if .PKGBUILD.PostRm}}
post_remove() {
  {{.PKGBUILD.PostRm}}"
}
{{- end }}
`
