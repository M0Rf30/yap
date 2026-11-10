// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pacman provides Arch Linux package building functionality and constants.
package pacman

// specFile renders the makepkg-compatible PKGBUILD kept beside the build tree.
const specFile = `
{{- if .Maintainer}}
# Maintainer: {{.Maintainer}}
{{- end }}
pkgname={{.PkgName}}
{{- if .Epoch}}
epoch={{.Epoch}}
{{- end }}
pkgver={{.PkgVer}}
pkgrel={{.PkgRel}}
pkgdesc="{{.PkgDesc}}"
{{- if .ArchComputed}}
arch=({{.ArchComputed}})
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
{{- if or .PreInst .PostInst .PreRm .PostRm .PreUpgrade .PostUpgrade}}
install={{.PkgName}}.install
{{- end }}

package() {
{{.Package}}
}
`
