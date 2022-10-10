package redhat

const specFile = `
{{- /* Mandatory fields */ -}}
Name: {{.Pack.PkgName}}
Summary: {{.Pack.PkgDesc}}
Version: {{.Pack.PkgVer}}
Release: {{.Pack.PkgRel}}
Group: {{.Pack.Section}}
{{- if .Pack.URL}}
URL: {{.Pack.URL}}
{{- end }}
{{- if .Pack.License}}
{{- with .Pack.License}}
License: {{join .}}
{{- end }}
{{- else }}
License: CUSTOM
{{- end }}
{{- if .Pack.Maintainer}}
Packager: {{.Pack.Maintainer}}
{{- end }}
{{- with .Pack.Provides}}
Provides: {{join .}}
{{- end }}
{{- with .Pack.Conflicts}}
Conflicts: {{join .}}
{{- end }}
{{- with .Pack.Depends}}
Requires: {{join .}}
{{- end }}
{{- with .Pack.MakeDepends}}
BuildRequires: {{join .}}
{{- end }}

%global _build_id_links none
%global _python_bytecompile_extra 0
%global _python_bytecompile_errors_terminate_build 0
%undefine __brp_python_bytecompile

%description
{{.Pack.PkgDesc}}

%install
rsync -a -A {{.Pack.PackageDir}}/ $RPM_BUILD_ROOT/

%files
{{- range $key, $value := .Files }}
{{- if $value }}
{{$value}}
{{- end }}
{{- end }}

{{- with .Pack.PreInst}}
%pre
{{.Pack.PreInst}}
{{- end }}

{{- with .Pack.PostInst}}
%post
{{.Pack.PostRm}}
{{- end }}

{{- with .Pack.PreRm}}
%preun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.Pack.PreRm}}
{{- end }}

{{- with .Pack.PostRm}}
%postun
if [[ "$1" -ne 0 ]]; then exit 0; fi
{{.Pack.PostRm}}
{{- end }}
`
const (
	Communications = "Applications/Communications"
	Engineering    = "Applications/Engineering"
	Internet       = "Applications/Internet"
	Multimedia     = "Applications/Multimedia"
	Tools          = "Development/Tools"
)
