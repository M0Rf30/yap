package pacman

var buildEnvironmentDeps = []string{
	"base-devel",
}

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
{{- with .Arch}}
arch=({{join .}})
{{- end }}
{{- with .Depends}}
depends=({{join .}})
{{- end }}
{{- with .OptDepends}}
optdepends=({{join .}})
{{- end }}
{{- /* Optional fields */ -}}
{{- with .Provides}}
provides=({{join .}})
{{- end }}
{{- with .Conflicts}}
conflicts=({{join .}})
{{- end }}
{{- if .URL}}
url="{{.URL}}"
{{- end }}
{{- if .Backup}}
backup=("{{join .}}")
{{- end }}
{{- with .License}}
license=({{join .}})
{{- end }}
{{- with .Options}}
options=({{join .}})
{{- end }}
install={{.PkgName}}.install

package() {{.Package}}
`
const postInstall = `
{{- if .PreInst}}
pre_install() {
  {{.PreInst}}"
}
{{- end }}

{{- if .PostInst}}
post_install() {
  {{.PostInst}}"
}
{{- end }}

{{- if .PreInst}}
pre_upgrade() {
  {{.PreInst}}"
}
{{- end }}

{{- if .PostInst}}
post_upgrade() {
  {{.PostInst}}"
}
{{- end }}

{{- if .PreRm}}
pre_remove() {
  {{.PreRm}}"
}
{{- end }}

{{- if .PostRm}}
post_remove() {
  {{.PostRm}}"
}
{{- end }}
`
