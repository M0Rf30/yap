package pacman

var buildEnvironmentDeps = []string{
	"base-devel",
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
# Maintainer: {{.Maintainer}}
{{- if .PkgDest}}
PKGDEST="{{.PkgDest}}"
{{- end }}

pkgname={{.PkgName}}
{{- if .Epoch}}
epoch={{.Epoch}}
{{- end }}
pkgver={{.PkgVer}}
pkgrel={{.PkgRel}}
pkgdesc="{{.PkgDesc}}"
{{- if .Arch}}
arch=(
  {{ range .Arch }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .Depends}}
depends=(
  {{ range .Depends }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .OptDepends}}
optdepends=(
  {{ range .OptDepends }}"{{ . }}"
  {{ end }}
)
{{- end }}

{{- /* Optional fields */ -}}
{{- if .Provides}}
provides=(
  {{ range .Provides }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .Conflicts}}
conflicts=(
  {{ range .Conflicts }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .URL}}
url="{{.URL}}"
{{- end }}
{{- if .Backup}}
backup=(
  {{ range .Backup }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .License}}
license=(
  {{ range .License }}"{{ . }}"
  {{ end }}
)
{{- end }}
{{- if .Options}}
options=(
  {{ range .Options }}"{{ . }}"
  {{ end }}
)
{{- end }}
install={{.PkgName}}.install

package() {
{{.Package}}
}
`
