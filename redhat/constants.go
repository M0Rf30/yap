package redhat

const (
	Communications = "Applications/Communications"
	Engineering    = "Applications/Engineering"
	Internet       = "Applications/Internet"
	Multimedia     = "Applications/Multimedia"
	Tools          = "Development/Tools"
)

var (
	RPMGroups = map[string]string{
		"admin":        "Applications/System",
		"any":          "noarch",
		"comm":         Communications,
		"database":     "Applications/Databases",
		"debug":        "Development/Debuggers",
		"devel":        Tools,
		"doc":          "Documentation",
		"editors":      "Applications/Editors",
		"electronics":  Engineering,
		"embedded":     Engineering,
		"fonts":        "Interface/Desktops",
		"games":        "Amusements/Games",
		"graphics":     Multimedia,
		"httpd":        Internet,
		"interpreters": Tools,
		"kernel":       "System Environment/Kernel",
		"libdevel":     "Development/Libraries",
		"libs":         "System Environment/Libraries",
		"localization": "Development/Languages",
		"mail":         Communications,
		"math":         "Applications/Productivity",
		"misc":         "Applications/System",
		"net":          Internet,
		"news":         "Applications/Publishing",
		"science":      Engineering,
		"shells":       "System Environment/Shells",
		"sound":        Multimedia,
		"text":         "Applications/Text",
		"vcs":          Tools,
		"video":        Multimedia,
		"web":          Internet,
		"x11":          "User Interface/X",
	}

	ArchToRPM = map[string]string{
		"any": "noarch",
	}
)

const specFile = `
{{- /* Mandatory fields */ -}}
Name: {{.PKGBUILD.PkgName}}
Summary: {{.PKGBUILD.PkgDesc}}
Version: {{.PKGBUILD.PkgVer}}
Release: {{.PKGBUILD.PkgRel}}
{{- if .PKGBUILD.Section}}
Group: {{.PKGBUILD.Section}}
{{- end }}
{{- if .PKGBUILD.URL}}
URL: {{.PKGBUILD.URL}}
{{- end }}
{{- if .PKGBUILD.License}}
{{- with .PKGBUILD.License}}
License: {{join .}}
{{- end }}
{{- else }}
License: CUSTOM
{{- end }}
{{- if .PKGBUILD.Maintainer}}
Packager: {{.PKGBUILD.Maintainer}}
{{- end }}
{{- with .PKGBUILD.Provides}}
Provides: {{join .}}
{{- end }}
{{- with .PKGBUILD.Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .PKGBUILD.Depends}}
Requires: {{join .}}
{{- end }}
{{- with .PKGBUILD.MakeDepends}}
BuildRequires: {{join .}}
{{- end }}

%global _build_id_links none
%global _python_bytecompile_extra 0
%global _python_bytecompile_errors_terminate_build 0
%undefine __brp_python_bytecompile

%description
{{.PKGBUILD.PkgDesc}}

%install
rsync -a -A {{.PKGBUILD.PackageDir}}/ $RPM_BUILD_ROOT/

%files
{{- range $key, $value := .Files }}
{{- if $value }}
{{$value}}
{{- end }}
{{- end }}

{{- with .PKGBUILD.PreInst}}
%pre
{{.PKGBUILD.PreInst}}
{{- end }}

{{- with .PKGBUILD.PostInst}}
%post
{{.PKGBUILD.PostRm}}
{{- end }}

{{- with .PKGBUILD.PreRm}}
%preun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.PKGBUILD.PreRm}}
{{- end }}

{{- with .PKGBUILD.PostRm}}
%postun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.PKGBUILD.PostRm}}
{{- end }}
`
