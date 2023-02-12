package pacman

const specFile = `
{{- /* Mandatory fields */ -}}
# Maintainer: {{.PKGBUILD.Maintainer}}
pkgname={{.PKGBUILD.PkgName}}
{{- with .PKGBUILD.Epoch}}
epoch={{.PKGBUILD.Epoch}}
{{- end }}
pkgver={{.PKGBUILD.PkgVer}}
pkgrel={{.PKGBUILD.PkgRel}}
pkgdesc="{{.PKGBUILD.PkgDesc}}"
{{- with .PKGBUILD.Arch}}
arch=({{join .}})
{{- end }}
{{- with .PKGBUILD.Depends}}
depends=({{join .}})
{{- end }}
{{- with .PKGBUILD.OptDepends}}
optdepends=({{join .}})
{{- end }}
{{- /* Optional fields */ -}}
{{- with .PKGBUILD.Provides}}
provides=({{join .}})
{{- end }}
{{- with .PKGBUILD.Conflicts}}
conflicts=({{join .}})
{{- end }}
{{- if .PKGBUILD.URL}}
url="{{.PKGBUILD.URL}}"
{{- end }}
{{- if .PKGBUILD.Backup}}
backup=("{{join .}}")
{{- end }}
{{- with .PKGBUILD.License}}
license=({{join .}})
{{- end }}
options=("emptydirs")
install={{.PKGBUILD.PkgName}}.install

package() {
  rsync -a -A {{.PKGBUILD.PackageDir}}/ ${pkgdir}/
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
