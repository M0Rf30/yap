package redhat

const (
	Communications = "Applications/Communications"
	Engineering    = "Applications/Engineering"
	Internet       = "Applications/Internet"
	Multimedia     = "Applications/Multimedia"
	Tools          = "Development/Tools"
)

var buildEnvironmentDeps = []string{
	"automake",
	"createrepo",
	"expect",
	"gcc",
	"make",
	"openssl",
	"rpm-build",
	"rpm-sign",
}

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
Name: {{.PkgName}}
Summary: {{.PkgDesc}}
Version: {{.PkgVer}}
Release: {{.PkgRel}}
{{- if .Section}}
Group: {{.Section}}
{{- end }}
{{- if .URL}}
URL: {{.URL}}
{{- end }}
{{- if .License}}
{{- with .License}}
License: {{join .}}
{{- end }}
{{- else }}
License: CUSTOM
{{- end }}
{{- if .Maintainer}}
Packager: {{.Maintainer}}
{{- end }}
{{- with .Provides}}
Provides: {{join .}}
{{- end }}
{{- with .Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .Depends}}
Requires: {{join .}}
{{- end }}
{{- with .MakeDepends}}
BuildRequires: {{join .}}
{{- end }}

{{- if .PkgDest}}
%define _rpmdir {{.PkgDest}}
{{- end }}

%global _build_id_links none
%global _python_bytecompile_extra 0
%global _python_bytecompile_errors_terminate_build 0
%undefine __brp_python_bytecompile

%description
{{.PkgDesc}}

%files
{{- range $key, $value := .Files }}
{{- if $value }}
{{$value}}
{{- end }}
{{- end }}

{{- with .PreInst}}
%pre
{{.PreInst}}
{{- end }}

{{- with .PostInst}}
%post
{{.PostRm}}
{{- end }}

{{- with .PreRm}}
%preun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.PreRm}}
{{- end }}

{{- with .PostRm}}
%postun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.PostRm}}
{{- end }}
`
