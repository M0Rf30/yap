package pacman

const specFile = `
{{- /* Mandatory fields */ -}}
# Maintainer: {{.Pack.Maintainer}}
pkgname={{.Pack.PkgName}}
{{- with .Pack.Epoch}}
epoch={{.Pack.Epoch}}
{{- end }}
pkgver={{.Pack.PkgVer}}
pkgrel={{.Pack.PkgRel}}
pkgdesc="{{.Pack.PkgDesc}}"
{{- with .Pack.Arch}}
arch=({{join .}})
{{- end }}
{{- with .Pack.Depends}}
depends=({{join .}})
{{- end }}
{{- with .Pack.OptDepends}}
optdepends=({{join .}})
{{- end }}
{{- /* Optional fields */ -}}
{{- with .Pack.Provides}}
provides=({{join .}})
{{- end }}
{{- with .Pack.Conflicts}}
conflicts=({{join .}})
{{- end }}
{{- if .Pack.URL}}
url="{{.Pack.URL}}"
{{- end }}
{{- if .Pack.Backup}}
backup=("{{join .}}")
{{- end }}
{{- with .Pack.License}}
license=({{join .}})
{{- end }}
options=("emptydirs")
install={{.Pack.PkgName}}.install

package() {
  rsync -a -A {{.Pack.PackageDir}}/ ${pkgdir}/
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
