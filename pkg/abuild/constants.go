package abuild

var installArgs = []string{
	"add",
	"--allow-untrusted",
}

var (
	APKArchs = map[string]string{
		"x86_64":  "x86_64",
		"i686":    "x86",
		"aarch64": "aarch64",
		"armv7h":  "armv7h",
		"armv6h":  "armv6h",
		"any":     "all",
	}

	buildEnvironmentDeps = []string{
		"alpine-sdk",
		"ccache",
	}
)

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
{{- if .ArchComputed}}
arch="{{.ArchComputed}}"
{{- end}}
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
