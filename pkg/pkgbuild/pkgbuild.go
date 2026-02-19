// Package pkgbuild provides PKGBUILD structure and manipulation functionality.
package pkgbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/github/go-spdx/v2/spdxexp"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/set"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// ArchAny represents architecture-independent packages.
const ArchAny = "any"

// FuncBody is a tagged string type used exclusively for PKGBUILD function
// bodies.  AddItem uses this type to distinguish function declarations from
// plain string variable values so that mapFunctions does not misidentify
// variables like "maintainer" as helper function definitions.
type FuncBody string

const (
	dependsKey = "depends"
)

// Priority constants for PKGBUILD directive matching.
// Higher values indicate a more specific (and therefore preferred) match.
const (
	// prioritySkip means this directive does not apply to the current system.
	prioritySkip = -1
	// priorityBase is the default priority for non-qualified directives.
	priorityBase = 0
	// priorityPackagerMatch is for directives matching the package manager (e.g. "apt").
	priorityPackagerMatch = 1
	// priorityDistroMatch is for directives matching the distro name (e.g. "ubuntu").
	priorityDistroMatch = 2
	// priorityFullDistroMatch is for directives matching distro+codename (e.g. "ubuntu_jammy").
	priorityFullDistroMatch = 3
	// priorityArchMatch is for architecture-specific directives.
	priorityArchMatch = 4
	// priorityArchDistroBoost is added to distro priority for arch+distro combined directives.
	priorityArchDistroBoost = 4
)

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions).
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch            []string
	ArchComputed    string
	Backup          []string
	Build           string
	BuildArch       string // Build architecture for cross-compilation (where compilation happens)
	BuildDate       int64
	Checksum        string
	Codename        string
	Commit          string
	Conflicts       []string
	Copyright       []string
	CustomArrays    map[string][]string
	CustomVariables map[string]string
	DataHash        string
	DebConfig       string
	DebTemplate     string
	Depends         []string
	Distro          string
	Epoch           string
	Files           []string
	FullDistroName  string
	Group           string
	HashSums        []string
	HelperFunctions map[string]string
	Home            string
	HostArch        string // Host architecture for cross-compilation (where package will run)
	Install         string
	InstalledSize   int64
	License         []string
	Maintainer      string
	MakeDepends     []string
	OptDepends      []string
	Options         []string
	Origin          string
	Package         string
	PackageDir      string
	PkgDesc         string
	PkgDest         string
	PkgName         string
	PkgRel          string
	PkgType         string
	PkgVer          string
	PostInst        string
	PostRm          string
	PostTrans       string
	PreInst         string
	Prepare         string
	PreRelease      string
	PreRm           string
	PreTrans        string
	priorities      map[string]int
	Priority        string
	Provides        []string
	Replaces        []string
	Section         string
	SourceDir       string
	SourceURI       []string
	StartDir        string
	TargetArch      string // Target architecture for cross-compilation (what we're building for)
	URL             string
	StaticEnabled   bool
	StripEnabled    bool
	YAPVersion      string
}

// AddItem adds an item to the PKGBUILD.
//
// It takes a key string and data of any type as parameters.
// It returns an error.
func (pkgBuild *PKGBUILD) AddItem(key string, data any) error {
	key, priority, err := pkgBuild.parseDirective(key)
	if err != nil || priority == -1 {
		return err
	}

	if priority < pkgBuild.priorities[key] {
		return nil
	}

	pkgBuild.priorities[key] = priority
	pkgBuild.mapVariables(key, data)
	pkgBuild.mapArrays(key, data)
	pkgBuild.mapFunctions(key, data)

	return nil
}

// ComputeArchitecture checks if the specified architecture is supported.
// If "any", sets to "any". Otherwise, checks if current architecture is supported.
// Logs error if not supported, then sets to current architecture if supported.
func (pkgBuild *PKGBUILD) ComputeArchitecture() {
	isSupported := set.Contains(pkgBuild.Arch, ArchAny)
	if isSupported {
		pkgBuild.ArchComputed = ArchAny

		return
	}

	currentArch := platform.GetArchitecture()

	isSupported = set.Contains(pkgBuild.Arch, currentArch)
	if !isSupported {
		logger.Fatal(i18n.T("errors.pkgbuild.unsupported_architecture"),
			"pkgname", pkgBuild.PkgName,
			"arch", strings.Join(pkgBuild.Arch, " "))
	}

	pkgBuild.ArchComputed = currentArch
}

// CreateSpec reads the filepath where the specfile will be written and the
// content of the specfile. Specfile generation is done using go templates for
// every different distro family. It returns any error if encountered.
func (pkgBuild *PKGBUILD) CreateSpec(filePath string, tmpl *template.Template) error {
	cleanFilePath := filepath.Clean(filePath)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createspec.warn.failed_to_close_pkgbuild_1"),
				"path", cleanFilePath,
				"error", err)
		}
	}()

	return tmpl.Execute(file, pkgBuild)
}

// filterInstalledPackages checks which packages are not installed and returns only those.
// It uses the appropriate package manager query command based on the package manager name.
func filterInstalledPackages(packageManager string, packages []string) []string {
	if len(packages) == 0 {
		return nil
	}

	var (
		queryCmd  string
		queryArgs []string
	)

	// Determine the query command based on package manager

	switch packageManager {
	case "pacman":
		queryCmd = "pacman"
		queryArgs = []string{"-Q"}
	case "dpkg":
		queryCmd = "dpkg"
		queryArgs = []string{"-s"}
	case "rpm":
		queryCmd = "rpm"
		queryArgs = []string{"-q"}
	case "apk":
		queryCmd = "apk"
		queryArgs = []string{"info", "-e"}
	default:
		// Unknown package manager, return all packages
		return packages
	}

	missingPackages := make([]string, 0, len(packages))

	// Check each package individually
	for _, pkg := range packages {
		// Create a new slice with the query args and package name
		args := make([]string, len(queryArgs)+1)
		copy(args, queryArgs)
		args[len(queryArgs)] = pkg

		err := shell.Exec(context.Background(), true, "", queryCmd, args...)
		if err != nil {
			// Package not installed or query failed, add to missing list
			missingPackages = append(missingPackages, pkg)
		}
	}

	return missingPackages
}

// GetDepends reads the package manager name, its arguments and all the
// dependencies required to build the package. It returns any error if
// encountered.
func (pkgBuild *PKGBUILD) GetDepends(packageManager string, args, makeDepends []string) error {
	if len(makeDepends) == 0 {
		return nil
	}

	// Filter out already-installed packages to avoid sudo prompts
	missingPackages := filterInstalledPackages(packageManager, makeDepends)

	// If all packages are already installed, nothing to do
	if len(missingPackages) == 0 {
		logger.Info(i18n.T("logger.pkgbuild.info.all_packages_installed"),
			"count", len(makeDepends))

		return nil
	}

	// Log which packages need installation
	if len(missingPackages) < len(makeDepends) {
		logger.Info(i18n.T("logger.pkgbuild.info.packages_already_installed"),
			"installed", len(makeDepends)-len(missingPackages),
			"missing", len(missingPackages))
	}

	args = append(args, missingPackages...)

	return shell.ExecWithSudo(context.Background(), false, "", packageManager, args...)
}

// GetUpdates reads the package manager name and its arguments to perform
// a sync with remotes and consequently retrieve updates.
// It returns any error if encountered.
func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	return shell.ExecWithSudo(context.Background(), false, "", packageManager, args...)
}

// Init initializes the PKGBUILD struct.
//
// It sets up the priorities map and assigns the full distribution name based on
// the Distro and Codename fields.
func (pkgBuild *PKGBUILD) Init() {
	pkgBuild.priorities = make(map[string]int)
	pkgBuild.CustomVariables = make(map[string]string)
	pkgBuild.CustomArrays = make(map[string][]string)
	pkgBuild.HelperFunctions = make(map[string]string)

	pkgBuild.FullDistroName = pkgBuild.Distro
	if pkgBuild.Codename != "" {
		pkgBuild.FullDistroName += "_" + pkgBuild.Codename
	}
}

// BuildScriptPreamble generates a bash preamble that declares custom scalar
// variables, custom array variables, and helper function definitions so that
// they are available inside build(), prepare(), and package() scripts.
//
// The preamble is prepended to every script body before execution.  Declarations
// are emitted in sorted order so that the output is deterministic.
func (pkgBuild *PKGBUILD) BuildScriptPreamble() string {
	var preamble strings.Builder

	// Emit custom scalar variables (e.g. _prefix="/usr")
	varKeys := make([]string, 0, len(pkgBuild.CustomVariables))
	for k := range pkgBuild.CustomVariables {
		varKeys = append(varKeys, k)
	}

	sort.Strings(varKeys)

	for _, k := range varKeys {
		preamble.WriteString(k)
		preamble.WriteString("='")
		preamble.WriteString(strings.ReplaceAll(pkgBuild.CustomVariables[k], "'", "'\\''"))
		preamble.WriteString("'\n")
	}

	// Emit custom array variables (e.g. _modules=('a' 'b'))
	arrKeys := make([]string, 0, len(pkgBuild.CustomArrays))
	for k := range pkgBuild.CustomArrays {
		arrKeys = append(arrKeys, k)
	}

	sort.Strings(arrKeys)

	for _, k := range arrKeys {
		preamble.WriteString(k)
		preamble.WriteString("=(")

		for idx, value := range pkgBuild.CustomArrays[k] {
			if idx > 0 {
				preamble.WriteByte(' ')
			}

			preamble.WriteByte('\'')
			preamble.WriteString(strings.ReplaceAll(value, "'", "'\\''"))
			preamble.WriteByte('\'')
		}

		preamble.WriteString(")\n")
	}

	// Emit helper function definitions in sorted order
	funcKeys := make([]string, 0, len(pkgBuild.HelperFunctions))
	for k := range pkgBuild.HelperFunctions {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	for _, k := range funcKeys {
		preamble.WriteString(pkgBuild.HelperFunctions[k])
		preamble.WriteByte('\n')
	}

	return preamble.String()
}

// HelperFunctionsPreamble returns a bash snippet containing only the helper
// function definitions that are referenced by the given scriptlet body.
// Unlike BuildScriptPreamble it does NOT emit custom scalar or array variable
// declarations, which are build-time values and have no meaning inside
// package-manager scriptlets (preinst, postinst, prerm, postrm).
//
// Only helpers whose names appear as call-sites in scriptletBody are included,
// so build-time helpers like _package or _package_systemd are not injected
// into install-time scripts.
func (pkgBuild *PKGBUILD) HelperFunctionsPreamble(scriptletBody string) string {
	if len(pkgBuild.HelperFunctions) == 0 {
		return ""
	}

	funcKeys := make([]string, 0, len(pkgBuild.HelperFunctions))
	for k := range pkgBuild.HelperFunctions {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	var preamble strings.Builder

	for _, k := range funcKeys {
		// Only include helpers that are actually called in the scriptlet body.
		// This prevents build-time helpers (e.g. _package, _package_systemd)
		// from being injected into package-manager scriptlets.
		if strings.Contains(scriptletBody, k) {
			preamble.WriteString(pkgBuild.HelperFunctions[k])
			preamble.WriteByte('\n')
		}
	}

	return preamble.String()
}

// RenderSpec initializes a new template with custom functions and parses the provided script.
// It adds two custom functions to the template:
//
//	"join": Takes a slice of strings and joins them into a single string, separated by commas,
//	while also trimming any leading or trailing spaces.
//	"multiline": Takes a string and replaces newline characters with a newline followed by a space,
//	effectively formatting the string for better readability in multi-line contexts.
//
// The method returns the parsed template, which can be used for rendering with data.
func (pkgBuild *PKGBUILD) RenderSpec(script string) *template.Template {
	tmpl := template.New("template").Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n ")

			return strings.Trim(ret, " \n")
		},
	})

	template.Must(tmpl.Parse(script))

	return tmpl
}

// SetMainFolders sets the main folders for the PKGBUILD.
//
// It takes no parameters.
// It does not return anything.
func (pkgBuild *PKGBUILD) SetMainFolders() {
	switch pkgBuild.Distro {
	case "alpine":
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", pkgBuild.PkgName)
	default:
		var folderName string
		if pkgBuild.Distro == "arch" {
			folderName = "pkg"
		} else {
			folderName = "pkg-" + pkgBuild.Distro
		}

		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, folderName)
	}
}

// BuildEnvironmentSlice returns the package-specific environment variables as a
// "KEY=VALUE" slice that can be merged with os.Environ() for safe concurrent use.
// Unlike SetEnvironmentVariables, it does NOT mutate the process environment, making
// it safe to call from multiple goroutines simultaneously (parallel builds).
//
// The returned slice includes:
//   - pkgdir, srcdir, startdir — per-package directory paths
//   - pkgname, pkgver, pkgrel — per-package identity fields
//   - CPATH, LIBRARY_PATH, PKG_CONFIG_PATH — sysroot search paths prepended to
//     their current global values
func (pkgBuild *PKGBUILD) BuildEnvironmentSlice() []string {
	sysrootDir := filepath.Join(filepath.Dir(pkgBuild.StartDir), "yap-sysroot")

	// Build sysroot path additions without touching the global env.
	cpathDirs := []string{
		filepath.Join(sysrootDir, "usr", "include"),
		filepath.Join(sysrootDir, "usr", "local", "include"),
	}

	libDirs := []string{
		filepath.Join(sysrootDir, "usr", "lib"),
		filepath.Join(sysrootDir, "usr", "local", "lib"),
	}

	pkgConfigDirs := []string{
		filepath.Join(sysrootDir, "usr", "lib", "pkgconfig"),
		filepath.Join(sysrootDir, "usr", "share", "pkgconfig"),
		filepath.Join(sysrootDir, "usr", "local", "lib", "pkgconfig"),
	}

	buildEnvPath := func(key string, prepend []string) string {
		existing := os.Getenv(key)
		parts := make([]string, 0, len(prepend)+1)
		parts = append(parts, prepend...)

		if existing != "" {
			parts = append(parts, existing)
		}

		return key + "=" + strings.Join(parts, ":")
	}

	return []string{
		"pkgdir=" + pkgBuild.PackageDir,
		"srcdir=" + pkgBuild.SourceDir,
		"startdir=" + pkgBuild.StartDir,
		"pkgname=" + pkgBuild.PkgName,
		"pkgver=" + pkgBuild.PkgVer,
		"pkgrel=" + pkgBuild.PkgRel,
		buildEnvPath("CPATH", cpathDirs),
		buildEnvPath("LIBRARY_PATH", libDirs),
		buildEnvPath("PKG_CONFIG_PATH", pkgConfigDirs),
	}
}

// SetEnvironmentVariables sets the environment variables for the PKGBUILD execution context.
// This should be called just before executing build/package functions to ensure
// each package uses its own directories, even when building multiple packages.
//
// NOTE: For parallel builds, prefer BuildEnvironmentSlice() which does not mutate
// the global process environment and is safe to call from multiple goroutines.
//
// It always sets up sysroot environment paths (CPATH, LIBRARY_PATH, PKG_CONFIG_PATH)
// pointing to yap-sysroot/ so that internal dependencies extracted there are visible
// to build scripts without mutating CFLAGS or LDFLAGS.
//
// It returns an error if setting any environment variable fails.
func (pkgBuild *PKGBUILD) SetEnvironmentVariables() error {
	err := os.Setenv("pkgdir", pkgBuild.PackageDir)
	if err != nil {
		return err
	}

	err = os.Setenv("srcdir", pkgBuild.SourceDir)
	if err != nil {
		return err
	}

	err = os.Setenv("startdir", pkgBuild.StartDir)
	if err != nil {
		return err
	}

	// Always refresh pkgname/pkgver/pkgrel so that scripts for each package
	// in a multi-package build see their own values, not stale ones left in
	// the environment by the previously built package.
	err = os.Setenv("pkgname", pkgBuild.PkgName)
	if err != nil {
		return err
	}

	err = os.Setenv("pkgver", pkgBuild.PkgVer)
	if err != nil {
		return err
	}

	err = os.Setenv("pkgrel", pkgBuild.PkgRel)
	if err != nil {
		return err
	}

	// Always set up sysroot environment so internal dependencies (extracted to
	// yap-sysroot/) are visible to build scripts via CPATH/LIBRARY_PATH/PKG_CONFIG_PATH.
	return pkgBuild.SetupSysrootEnvironment()
}

// SetupSysrootEnvironment configures CPATH, LIBRARY_PATH, and PKG_CONFIG_PATH
// to include the yap-sysroot directory derived from this PKGBUILD's StartDir.
// It does NOT mutate CFLAGS or LDFLAGS, so user PKGBUILD scripts that set
// CFLAGS="-O2" still benefit from sysroot headers.
func (pkgBuild *PKGBUILD) SetupSysrootEnvironment() error {
	sysrootDir := filepath.Join(filepath.Dir(pkgBuild.StartDir), "yap-sysroot")

	includeDirs := []string{
		filepath.Join(sysrootDir, "usr", "include"),
		filepath.Join(sysrootDir, "usr", "local", "include"),
	}

	libDirs := []string{
		filepath.Join(sysrootDir, "usr", "lib"),
		filepath.Join(sysrootDir, "usr", "local", "lib"),
	}

	pkgConfigDirs := []string{
		filepath.Join(sysrootDir, "usr", "lib", "pkgconfig"),
		filepath.Join(sysrootDir, "usr", "share", "pkgconfig"),
		filepath.Join(sysrootDir, "usr", "local", "lib", "pkgconfig"),
	}

	if err := prependEnvPaths("CPATH", includeDirs); err != nil {
		return err
	}

	if err := prependEnvPaths("LIBRARY_PATH", libDirs); err != nil {
		return err
	}

	return prependEnvPaths("PKG_CONFIG_PATH", pkgConfigDirs)
}

// prependEnvPaths prepends dirs to the colon-separated environment variable named by key.
func prependEnvPaths(key string, dirs []string) error {
	existing := os.Getenv(key)
	parts := make([]string, 0, len(dirs)+1)
	parts = append(parts, dirs...)

	if existing != "" {
		parts = append(parts, existing)
	}

	return os.Setenv(key, strings.Join(parts, ":"))
}

// ValidateGeneral checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateGeneral() error {
	var checkErrors []string

	// Validate license
	if !pkgBuild.checkLicense() {
		checkErrors = append(checkErrors, "license")

		logger.Error(i18n.T("logger.validategeneral.error.invalid_spdx_license_identifier_1"),
			"pkgname", pkgBuild.PkgName)
		logger.Info(i18n.T("logger.validategeneral.info.you_can_find_valid_1"),
			"🌐", "https://spdx.org/licenses/")
	}

	// Check source and hash sums
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		checkErrors = append(checkErrors, "source-hash mismatch")

		logger.Error(i18n.T("logger.validategeneral.error.number_of_sources_and_1"),
			"pkgname", pkgBuild.PkgName)
	}

	// Check for package() function
	if pkgBuild.Package == "" {
		checkErrors = append(checkErrors, "package function")

		logger.Error(i18n.T("logger.validategeneral.error.missing_package_function_1"),
			"pkgname", pkgBuild.PkgName)
	}

	// Return error if there are validation errors
	if len(checkErrors) > 0 {
		return fmt.Errorf("pkgbuild validation failed for %q: %s",
			pkgBuild.PkgName, strings.Join(checkErrors, ", "))
	}

	return nil
}

// ValidateMandatoryItems checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateMandatoryItems() error {
	var validationErrors []string

	// Check mandatory variables
	mandatoryChecks := map[string]string{
		"pkgdesc": pkgBuild.PkgDesc,
		"pkgname": pkgBuild.PkgName,
		"pkgrel":  pkgBuild.PkgRel,
		"pkgver":  pkgBuild.PkgVer,
	}

	for variable, value := range mandatoryChecks {
		if value == "" {
			validationErrors = append(validationErrors, variable)
		}
	}

	// Return error if there are validation errors
	if len(validationErrors) > 0 {
		return fmt.Errorf("missing mandatory variables: %s", strings.Join(validationErrors, ", "))
	}

	return nil
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
//
//nolint:gocyclo,cyclop // Central dispatch function for PKGBUILD array field mapping
func (pkgBuild *PKGBUILD) mapArrays(key string, data any) {
	if pkgBuild.mapChecksumsArrays(key, data) {
		return
	}

	switch key {
	case "arch":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Arch = arrVal
	case "copyright":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Copyright = arrVal
	case "license":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.License = arrVal
	case dependsKey:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Depends = arrVal
	case "options":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Options = arrVal
		pkgBuild.processOptions()
	case "optdepends":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.OptDepends = arrVal
	case "makedepends":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.MakeDepends = arrVal
	case "provides":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Provides = arrVal
	case "conflicts":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Conflicts = arrVal
	case "replaces":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Replaces = arrVal
	case "source":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.SourceURI = arrVal
	case "backup":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Backup = arrVal
	default:
		// Store unknown arrays (e.g. _modules, _extra_files) as custom arrays.
		// They will be declared as bash array variables in the build script preamble.
		if arrVal, ok := data.([]string); ok {
			if pkgBuild.CustomArrays == nil {
				pkgBuild.CustomArrays = make(map[string][]string)
			}

			pkgBuild.CustomArrays[key] = arrVal
		}
	}
}

// mapChecksumsArrays handles mapping of checksum arrays and returns true if handled
func (pkgBuild *PKGBUILD) mapChecksumsArrays(key string, data any) bool {
	switch key {
	case "sha512sums", "sha384sums", "sha256sums", "sha224sums", "b2sums", "cksums":
		if arrVal, ok := data.([]string); ok {
			pkgBuild.HashSums = arrVal
		}

		return true
	default:
		return false
	}
}

// mapFunctions reads a function name and its content and maps them to the
// PKGBUILD struct. Any function name not matching a known PKGBUILD lifecycle
// hook is stored as a helper function and will be injected as a preamble into
// build, prepare, and package scripts so that callers such as _package() or
// _install_files() are available at runtime.
//
// data must be of type FuncBody; plain string values (variables) are ignored so
// that scalar variables such as "maintainer" are not confused with functions.
func (pkgBuild *PKGBUILD) mapFunctions(key string, data any) {
	fb, ok := data.(FuncBody)
	if !ok {
		return
	}

	body := string(fb)

	switch key {
	case "build":
		pkgBuild.Build = body
	case "package":
		pkgBuild.Package = body
	case "preinst":
		pkgBuild.PreInst = body
	case "prepare":
		pkgBuild.Prepare = body
	case "postinst":
		pkgBuild.PostInst = body
	case "posttrans":
		pkgBuild.PostTrans = body
	case "prerm":
		pkgBuild.PreRm = body
	case "pretrans":
		pkgBuild.PreTrans = body
	case "postrm":
		pkgBuild.PostRm = body
	default:
		// Store any other function (e.g. _package, _package_systemd, _install_helper)
		// as a helper. The full definition is reconstructed so it can be prepended to
		// build scripts verbatim.
		if pkgBuild.HelperFunctions == nil {
			pkgBuild.HelperFunctions = make(map[string]string)
		}

		pkgBuild.HelperFunctions[key] = fmt.Sprintf("%s() {\n%s\n}", key, body)
	}
}

// mapVariables reads a variable name and its content and maps them to the
// PKGBUILD struct.
//
//nolint:gocyclo,cyclop // Central dispatch function for PKGBUILD field mapping
func (pkgBuild *PKGBUILD) mapVariables(key string, data any) {
	var err error

	switch key {
	case "pkgname":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgName = strVal
	case "epoch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.Epoch = strVal
	case "pkgver":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgVer = strVal
	case "pkgrel":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgRel = strVal
	case "pkgdesc":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.PkgDesc = strVal
	case "maintainer":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.Maintainer = strVal
	case "section":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Section = strVal
	case "priority":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Priority = strVal
	case "url":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.URL = strVal
	case "origin":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Origin = strVal
	case "commit":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Commit = strVal
	case "debconf_template":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.DebTemplate = strVal
	case "debconf_config":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.DebConfig = strVal
	case "install":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Install = strVal
	case "target_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.TargetArch = strVal
	case "build_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.BuildArch = strVal
	case "host_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.HostArch = strVal
	default:
		// Store unknown scalar variables (e.g. _prefix, _destdir) as custom variables
		// and expose them as environment variables so they are available in build scripts
		// via the sh interpreter's inherited environment.
		if strVal, ok := data.(string); ok {
			if pkgBuild.CustomVariables == nil {
				pkgBuild.CustomVariables = make(map[string]string)
			}

			pkgBuild.CustomVariables[key] = strVal

			err = os.Setenv(key, strVal)
		}
	}

	if err != nil {
		logger.Fatal(i18n.T("errors.pkgbuild.failed_to_set_variable"),
			"variable", key)
	}
}

// parseDirective is a function that takes an input string and returns a key,
// priority, and an error.
func (pkgBuild *PKGBUILD) parseDirective(input string) (key string, priority int, err error) {
	// Check for combined architecture + distribution syntax first (_arch__distro)
	if key, priority, found := pkgBuild.parseCombinedArchDistro(input); found {
		return key, priority, nil
	}

	// Check for architecture-specific syntax (single underscore, no double underscore)
	if key, priority, found := pkgBuild.parseArchitectureOnly(input); found {
		return key, priority, nil
	}

	// Parse distribution-only syntax
	return pkgBuild.parseDistributionOnly(input)
}

// parseCombinedArchDistro handles combined architecture + distribution syntax
func (pkgBuild *PKGBUILD) parseCombinedArchDistro(input string) (
	key string, priority int, found bool,
) {
	if !strings.Contains(input, "_") || !strings.Contains(input, "__") {
		return "", 0, false
	}

	parts := strings.SplitN(input, "__", 2)
	if len(parts) != 2 {
		return "", 0, false
	}

	archPart := parts[0]
	distributionPart := parts[1]

	if !strings.Contains(archPart, "_") {
		return "", 0, false
	}

	archSplit := strings.Split(archPart, "_")
	if len(archSplit) < 2 {
		return "", 0, false
	}

	possibleArch := strings.Join(archSplit[1:], "_")
	key = archSplit[0]

	if !pkgBuild.isValidArchitecture(possibleArch) {
		return "", 0, false
	}

	currentArch := platform.GetArchitecture()
	if possibleArch != currentArch {
		return key, prioritySkip, true // Invalid architecture for current system
	}

	// Check distribution part
	distPriority := pkgBuild.getDistributionPriority(distributionPart)
	if distPriority > priorityBase {
		return key, distPriority + priorityArchDistroBoost, true // Add boost for arch+distro combinations
	}

	return key, prioritySkip, true
}

// parseArchitectureOnly handles architecture-specific syntax
func (pkgBuild *PKGBUILD) parseArchitectureOnly(input string) (
	key string, priority int, found bool,
) {
	if !strings.Contains(input, "_") || strings.Contains(input, "__") {
		return "", 0, false
	}

	archSplit := strings.Split(input, "_")
	if len(archSplit) < 2 {
		return "", 0, false
	}

	possibleArch := strings.Join(archSplit[1:], "_")
	key = archSplit[0]

	if !pkgBuild.isValidArchitecture(possibleArch) {
		return "", 0, false
	}

	currentArch := platform.GetArchitecture()
	if possibleArch == currentArch {
		return key, priorityArchMatch, true // Higher priority than distribution-specific
	}

	return key, prioritySkip, true // Invalid architecture for current system
}

// parseDistributionOnly handles distribution-only syntax
func (pkgBuild *PKGBUILD) parseDistributionOnly(input string) (
	key string, priority int, err error,
) {
	split := strings.Split(input, "__")
	key = split[0]

	if len(split) == 1 {
		return key, priorityBase, nil
	}

	if len(split) != 2 {
		return key, priorityBase, fmt.Errorf(i18n.T("errors.pkgbuild.invalid_directive_use"), input)
	}

	if pkgBuild.FullDistroName == "" {
		return key, priorityBase, nil
	}

	directive := split[1]
	priority = pkgBuild.getDistributionPriority(directive)

	return key, priority, nil
}

// getDistributionPriority returns the priority for a distribution directive
func (pkgBuild *PKGBUILD) getDistributionPriority(directive string) int {
	switch {
	case directive == pkgBuild.FullDistroName:
		return priorityFullDistroMatch
	case constants.DistrosSet.Contains(directive) &&
		directive == pkgBuild.Distro:
		return priorityDistroMatch
	case constants.PackagersSet.Contains(directive) &&
		directive == constants.DistroPackageManager[pkgBuild.Distro]:
		return priorityPackagerMatch
	default:
		return prioritySkip
	}
}

// checkLicense checks if the license of the PKGBUILD is valid.
//
// This function takes no parameters.
//
// It first checks if the license is either "PROPRIETARY" or "CUSTOM". If it is,
// the function returns true, indicating that the license is valid.
//
// If the license is not one of the above, the function uses the spdxexp package
// to validate the license.
//
// Returns a boolean indicating if the license is valid.
func (pkgBuild *PKGBUILD) checkLicense() bool {
	for _, license := range pkgBuild.License {
		if license == "PROPRIETARY" || license == "CUSTOM" {
			return true
		}
	}

	isValid, _ := spdxexp.ValidateLicenses(pkgBuild.License)

	return isValid
}

func (pkgBuild *PKGBUILD) processOptions() {
	// Initialize all flags to true
	pkgBuild.StaticEnabled = true
	pkgBuild.StripEnabled = true

	// Iterate over the features array
	for _, option := range pkgBuild.Options {
		switch {
		case strings.HasPrefix(option, "!strip"):
			pkgBuild.StripEnabled = false
		case strings.HasPrefix(option, "!staticlibs"):
			pkgBuild.StaticEnabled = false
		}
	}
}

// isValidArchitecture checks if the provided architecture string is a valid architecture.
func (pkgBuild *PKGBUILD) isValidArchitecture(arch string) bool {
	validArchitectures := []string{
		"x86_64", "i686", "aarch64", "armv7h", "armv6h", "armv5",
		"ppc64", "ppc64le", "s390x", "mips", "mipsle", "riscv64",
		"pentium4", // Arch Linux 32 support
		ArchAny,    // Architecture-independent packages
	}

	return set.Contains(validArchitectures, arch)
}

// IsCrossCompilation checks if cross-compilation is enabled for this PKGBUILD.
// Returns true if any of the cross-compilation architecture variables are set.
func (pkgBuild *PKGBUILD) IsCrossCompilation() bool {
	return pkgBuild.TargetArch != "" || pkgBuild.BuildArch != "" || pkgBuild.HostArch != ""
}

// GetTargetArchitecture returns the target architecture for cross-compilation.
// If a target architecture is explicitly set in the PKGBUILD, it returns that.
// Otherwise, it returns the computed architecture from the standard architecture processing.
func (pkgBuild *PKGBUILD) GetTargetArchitecture() string {
	if pkgBuild.TargetArch != "" {
		return pkgBuild.TargetArch
	}

	return pkgBuild.ArchComputed
}
