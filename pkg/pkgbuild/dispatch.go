// Package pkgbuild provides PKGBUILD structure and manipulation functionality.
package pkgbuild

// scalarHandler defines how to handle a scalar (string) PKGBUILD variable.
// The apply function assigns the value to the appropriate PKGBUILD field.
//
// DECISION: os.Setenv calls have been removed from all handlers.
// Rationale:
//  1. The only consumers of these env vars (pkgname, pkgver, pkgrel, etc.) are in tests.
//  2. The parallel build path explicitly avoids global env mutation (see pkg/shell/exec.go).
//  3. BuildEnvironmentSlice() provides a safe, concurrent-friendly alternative for
//     passing variables to shell scripts via environment slices.
//  4. Removing os.Setenv eliminates race conditions in parallel builds.
type scalarHandler struct {
	apply func(*PKGBUILD, string)
}

// arrayHandler defines how to handle an array PKGBUILD variable.
type arrayHandler func(*PKGBUILD, []string, int)

// scalarHandlers maps PKGBUILD scalar variable names to their handlers.
// Note: os.Setenv calls have been removed from all handlers to avoid race conditions
// with parallel builds. The shell execution layer (pkg/shell/exec.go) uses
// BuildEnvironmentSlice() to pass variables safely via environment slices.
var scalarHandlers = map[string]scalarHandler{
	pkgnameKey: {
		apply: func(p *PKGBUILD, v string) { p.PkgName = v },
	},
	pkgbaseKey: {
		apply: func(p *PKGBUILD, v string) { p.PkgBase = v },
	},
	"epoch": {
		apply: func(p *PKGBUILD, v string) { p.Epoch = v },
	},
	pkgverKey: {
		apply: func(p *PKGBUILD, v string) { p.PkgVer = v },
	},
	pkgrelKey: {
		apply: func(p *PKGBUILD, v string) { p.PkgRel = v },
	},
	pkgdescKey: {
		apply: func(p *PKGBUILD, v string) { p.PkgDesc = v },
	},
	"maintainer": {
		apply: func(p *PKGBUILD, v string) { p.Maintainer = v },
	},
	"section": {
		apply: func(p *PKGBUILD, v string) { p.Section = v },
	},
	"priority": {
		apply: func(p *PKGBUILD, v string) { p.Priority = v },
	},
	"url": {
		apply: func(p *PKGBUILD, v string) { p.URL = v },
	},
	"origin": {
		apply: func(p *PKGBUILD, v string) { p.Origin = v },
	},
	"commit": {
		apply: func(p *PKGBUILD, v string) { p.Commit = v },
	},
	"debconf_template": {
		apply: func(p *PKGBUILD, v string) { p.DebTemplate = v },
	},
	"debconf_config": {
		apply: func(p *PKGBUILD, v string) { p.DebConfig = v },
	},
	"install": {
		apply: func(p *PKGBUILD, v string) { p.Install = v },
	},
	"changelog": {
		apply: func(p *PKGBUILD, v string) { p.Changelog = v },
	},
	"target_arch": {
		apply: func(p *PKGBUILD, v string) {
			// CLI flag takes precedence: only apply PKGBUILD-file value when not already set.
			if p.TargetArch == "" {
				p.TargetArch = v
			}
		},
	},
	"build_arch": {
		apply: func(p *PKGBUILD, v string) { p.BuildArch = v },
	},
	"host_arch": {
		apply: func(p *PKGBUILD, v string) { p.HostArch = v },
	},
}

// arrayHandlers maps PKGBUILD array variable names to their handlers.
var arrayHandlers = map[string]arrayHandler{
	archDistro: func(p *PKGBUILD, v []string, _ int) {
		p.Arch = v
	},
	"copyright": func(p *PKGBUILD, v []string, _ int) {
		p.Copyright = v
	},
	licenseKey: func(p *PKGBUILD, v []string, _ int) {
		p.License = v
	},
	dependsKey: func(p *PKGBUILD, v []string, _ int) {
		p.Depends = v
	},
	"options": func(p *PKGBUILD, v []string, _ int) {
		p.Options = v
		p.processOptions()
	},
	"optdepends": func(p *PKGBUILD, v []string, _ int) {
		p.OptDepends = v
	},
	makedependsKey: func(p *PKGBUILD, v []string, _ int) {
		p.MakeDepends = v
	},
	"provides": func(p *PKGBUILD, v []string, _ int) {
		p.Provides = v
	},
	"conflicts": func(p *PKGBUILD, v []string, _ int) {
		p.Conflicts = v
	},
	"enhances": func(p *PKGBUILD, v []string, _ int) {
		p.Enhances = v
	},
	"supplements": func(p *PKGBUILD, v []string, _ int) {
		p.Supplements = v
	},
	"replaces": func(p *PKGBUILD, v []string, _ int) {
		p.Replaces = v
	},
	sourceKey: func(p *PKGBUILD, v []string, priority int) {
		if priority > priorityBase {
			// Arch-specific source_<arch>: accumulate separately; merged by Finalize().
			p.archSourceURI = append(p.archSourceURI, v...)
		} else {
			p.SourceURI = v
		}
	},
	noextractKey: func(p *PKGBUILD, v []string, _ int) {
		p.NoExtract = v
	},
	"backup": func(p *PKGBUILD, v []string, _ int) {
		p.Backup = v
	},
	pkgnameKey: func(p *PKGBUILD, v []string, _ int) {
		// Split-package form: pkgname=('foo' 'bar')
		// Store the list; PkgName is set to the first entry so single-package
		// code paths continue to work unchanged.
		p.PkgNames = v
		if len(v) > 0 {
			p.PkgName = v[0]
		}
	},
}

// mapVariables reads a variable name and its content and maps them to the
// PKGBUILD struct.
//
// This is the table-driven dispatch version that replaces the original
// switch-statement implementation. It handles both known PKGBUILD variables
// and custom variables (stored in CustomVariables).
//
// Note: os.Setenv calls have been removed to avoid race conditions with
// parallel builds. The shell execution layer uses BuildEnvironmentSlice()
// to pass variables safely via environment slices.
func (pkgBuild *PKGBUILD) mapVariables(key string, data any) {
	strVal, ok := data.(string)
	if !ok {
		return
	}

	// Check if this is a known scalar variable
	if handler, exists := scalarHandlers[key]; exists {
		handler.apply(pkgBuild, strVal)
		return
	}

	// Store unknown scalar variables (e.g. _prefix, _destdir) as custom variables
	// and expose them as environment variables so they are available in build scripts
	// via the sh interpreter's inherited environment.
	if pkgBuild.CustomVariables == nil {
		pkgBuild.CustomVariables = make(map[string]string)
	}

	pkgBuild.CustomVariables[key] = strVal
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
//
// mapArrays maps an array key+value into the PKGBUILD struct.
// priority is forwarded so that arch-specific source/checksum arrays
// (priority > priorityBase) are APPENDED to the base arrays rather than
// replacing them — matching makepkg behaviour where source_aarch64 is
// concatenated onto source, not a replacement.
//
// This is the table-driven dispatch version that replaces the original
// switch-statement implementation.
func (pkgBuild *PKGBUILD) mapArrays(key string, data any, priority int) {
	if pkgBuild.mapChecksumsArrays(key, data, priority) {
		return
	}

	arrVal, ok := data.([]string)
	if !ok {
		return
	}

	// Check if this is a known array variable
	if handler, exists := arrayHandlers[key]; exists {
		handler(pkgBuild, arrVal, priority)
		return
	}

	// Store unknown arrays (e.g. _modules, _extra_files) as custom arrays.
	// They will be declared as bash array variables in the build script preamble.
	if pkgBuild.CustomArrays == nil {
		pkgBuild.CustomArrays = make(map[string][]string)
	}

	pkgBuild.CustomArrays[key] = arrVal
}
