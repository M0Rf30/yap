package pkgbuild

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/github/go-spdx/v2/spdxexp"
	"github.com/pkg/errors"
)

// FuncBody is a distinct type for function body strings, used to distinguish
// function declarations from scalar variable assignments in AddItem.
type FuncBody string

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions). It adds some exotics fields to manage debconfig
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch            []string
	ArchComputed    string
	Backup          []string
	Build           string
	BuildDate       int64
	Checksum        string
	Codename        string
	Conflicts       []string
	Copyright       []string
	CustomArrays    map[string][]string
	CustomVariables map[string]string
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
	Install         string
	InstalledSize   int64
	License         []string
	Maintainer      string
	MakeDepends     []string
	OptDepends      []string
	Options         []string
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
	pkgBuild.processOptions()

	return nil
}

// ComputeArchitecture checks if the specified architecture is supported.
// If "any", sets to "any". Otherwise, checks if current architecture is supported.
// Logs error if not supported, then sets to current architecture if supported.
func (pkgBuild *PKGBUILD) ComputeArchitecture() {
	isSupported := osutils.Contains(pkgBuild.Arch, "any")
	if isSupported {
		pkgBuild.ArchComputed = "any"

		return
	}

	currentArch := osutils.GetArchitecture()

	isSupported = osutils.Contains(pkgBuild.Arch, currentArch)
	if !isSupported {
		osutils.Logger.Fatal("unsupported architecture",
			osutils.Logger.Args(
				"pkgname", pkgBuild.PkgName,
				"arch", strings.Join(pkgBuild.Arch, " ")))
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
	defer file.Close()

	return tmpl.Execute(file, pkgBuild)
}

// GetDepends reads the package manager name, its arguments and all the
// dependencies required to build the package. It returns any error if
// encountered.
func (pkgBuild *PKGBUILD) GetDepends(packageManager string, args, makeDepends []string) error {
	if len(makeDepends) == 0 {
		return nil
	}

	args = append(args, makeDepends...)

	return osutils.Exec(false, "", packageManager, args...)
}

// GetUpdates reads the package manager name and its arguments to perform
// a sync with remotes and consequently retrieve updates.
// It returns any error if encountered.
func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	return osutils.Exec(false, "", packageManager, args...)
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

// PrepareScriptlets prepends helper function definitions to all non-empty
// scriptlet fields (preinst, postinst, prerm, postrm, pretrans, posttrans).
// Only helper functions that are actually referenced by a scriptlet are
// included, to avoid injecting build-time helpers into install-time scripts.
func (pkgBuild *PKGBUILD) PrepareScriptlets() {
	if len(pkgBuild.HelperFunctions) == 0 {
		return
	}

	scriptlets := []*string{
		&pkgBuild.PreInst,
		&pkgBuild.PostInst,
		&pkgBuild.PreRm,
		&pkgBuild.PostRm,
		&pkgBuild.PreTrans,
		&pkgBuild.PostTrans,
	}

	for _, scriptlet := range scriptlets {
		if *scriptlet == "" {
			continue
		}

		preamble := pkgBuild.scriptletPreamble(*scriptlet)
		if preamble != "" {
			*scriptlet = preamble + *scriptlet
		}
	}
}

// FormatScriptlets applies shfmt-style formatting (2-space indent, switch-case
// indent) to all non-empty scriptlet fields.
func (pkgBuild *PKGBUILD) FormatScriptlets() {
	scriptlets := []*string{
		&pkgBuild.PreInst,
		&pkgBuild.PostInst,
		&pkgBuild.PreRm,
		&pkgBuild.PostRm,
		&pkgBuild.PreTrans,
		&pkgBuild.PostTrans,
	}

	for _, scriptlet := range scriptlets {
		if *scriptlet != "" {
			*scriptlet = osutils.FormatScript(*scriptlet)
		}
	}
}

// BuildScriptPreamble generates a bash preamble that declares custom arrays
// and helper functions so they are available in build/package scripts.
func (pkgBuild *PKGBUILD) BuildScriptPreamble() string {
	var preamble strings.Builder

	// Emit custom arrays as bash array declarations
	keys := make([]string, 0, len(pkgBuild.CustomArrays))
	for k := range pkgBuild.CustomArrays {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
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

	preamble.WriteString(pkgBuild.helperFunctionsPreamble())

	return preamble.String()
}

// RenderSpec initializes a new template with custom functions and parses the provided script.
// It adds two custom functions to the template:
//  1. "join": Takes a slice of strings and joins them into a single string, separated by commas,
//     while also trimming any leading or trailing spaces.
//  2. "multiline": Takes a string and replaces newline characters with a newline followed by a space,
//     effectively formatting the string for better readability in multi-line contexts.
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

// CleanEnvironment removes all environment variables that were set during
// PKGBUILD parsing (known env keys, custom variables, and folder paths).
// This must be called before parsing a new project's PKGBUILD in multi-project
// builds to prevent variable values from leaking between projects.
func (pkgBuild *PKGBUILD) CleanEnvironment() {
	// Remove known env keys set by mapVariables.
	for _, key := range []string{
		"epoch", "maintainer", "pkgname",
		"pkgrel", "pkgver", "url",
	} {
		os.Unsetenv(key)
	}

	// Remove custom variables set by mapVariables.
	for key := range pkgBuild.CustomVariables {
		os.Unsetenv(key)
	}

	// Remove folder paths set by SetMainFolders.
	os.Unsetenv("pkgdir")
	os.Unsetenv("srcdir")
	os.Unsetenv("startdir")
}

// SetEnvironmentVariables sets all PKGBUILD environment variables that build
// scripts may reference at runtime. This must be called before executing any
// build function to ensure the correct per-project values are available,
// especially in multi-project builds where env vars may have been overwritten
// by a later project's parsing phase.
func (pkgBuild *PKGBUILD) SetEnvironmentVariables() error {
	vars := map[string]string{
		"pkgdir":   pkgBuild.PackageDir,
		"srcdir":   pkgBuild.SourceDir,
		"startdir": pkgBuild.StartDir,
		"pkgname":  pkgBuild.PkgName,
		"pkgver":   pkgBuild.PkgVer,
		"pkgrel":   pkgBuild.PkgRel,
		"epoch":    pkgBuild.Epoch,
		"url":      pkgBuild.URL,
	}

	for key, value := range vars {
		err := os.Setenv(key, value)
		if err != nil {
			return err
		}
	}

	// Also restore custom variables so helper functions can reference them.
	for key, value := range pkgBuild.CustomVariables {
		err := os.Setenv(key, value)
		if err != nil {
			return err
		}
	}

	return nil
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
		key := make([]byte, 5)

		_, err := rand.Read(key)
		if err != nil {
			osutils.Logger.Fatal("fatal error",
				osutils.Logger.Args("error", err))
		}

		randomString := hex.EncodeToString(key)
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, pkgBuild.Distro+"-"+randomString)
	}

	err := os.Setenv("pkgdir", pkgBuild.PackageDir)
	if err != nil {
		osutils.Logger.Fatal("failed to set variable pkgdir")
	}

	err = os.Setenv("srcdir", pkgBuild.SourceDir)
	if err != nil {
		osutils.Logger.Fatal("failed to set variable srcdir")
	}

	err = os.Setenv("startdir", pkgBuild.StartDir)
	if err != nil {
		osutils.Logger.Fatal("failed to set variable startdir")
	}

	err = osutils.SetupCcache()
	if err != nil {
		osutils.Logger.Fatal("failed to setup ccache",
			osutils.Logger.Args("error", err))
	}
}

// ValidateGeneral checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateGeneral() {
	var checkErrors []string

	// Validate license
	if !pkgBuild.checkLicense() {
		checkErrors = append(checkErrors, "license")

		osutils.Logger.Error("invalid SPDX license identifier",
			osutils.Logger.Args("pkgname", pkgBuild.PkgName))
		osutils.Logger.Info("you can find valid SPDX license identifiers here",
			osutils.Logger.Args("🌐", "https://spdx.org/licenses/"))
	}

	// Check source and hash sums
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		checkErrors = append(checkErrors, "source-hash mismatch")

		osutils.Logger.Error("number of sources and hashsums differs",
			osutils.Logger.Args("pkgname", pkgBuild.PkgName))
	}

	// Check for package() function
	if pkgBuild.Package == "" {
		checkErrors = append(checkErrors, "package function")

		osutils.Logger.Error("missing package() function",
			osutils.Logger.Args("pkgname", pkgBuild.PkgName))
	}

	// Exit if there are validation errors
	if len(checkErrors) > 0 {
		os.Exit(1)
	}
}

// ValidateMandatoryItems checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateMandatoryItems() {
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

	// Exit if there are validation errors
	if len(validationErrors) > 0 {
		osutils.Logger.Fatal(
			"failed to set variables",
			osutils.Logger.Args(
				"variables",
				strings.Join(validationErrors, " ")))
	}
}

// helperFunctionsPreamble returns helper function definitions as a bash string.
func (pkgBuild *PKGBUILD) helperFunctionsPreamble() string {
	if len(pkgBuild.HelperFunctions) == 0 {
		return ""
	}

	var result strings.Builder

	funcKeys := make([]string, 0, len(pkgBuild.HelperFunctions))
	for k := range pkgBuild.HelperFunctions {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	for _, k := range funcKeys {
		result.WriteString(pkgBuild.HelperFunctions[k])
		result.WriteByte('\n')
	}

	return result.String()
}

// scriptletPreamble returns only the helper function definitions that are
// referenced (directly or transitively) by the given scriptlet body.
func (pkgBuild *PKGBUILD) scriptletPreamble(body string) string {
	// Collect all helper function names that appear in the body,
	// then transitively include any helpers called by those helpers.
	needed := make(map[string]bool)
	queue := make([]string, 0)

	// Seed: find helpers referenced in the scriptlet body
	for name := range pkgBuild.HelperFunctions {
		if strings.Contains(body, name) {
			needed[name] = true
			queue = append(queue, name)
		}
	}

	// Transitive closure: check helper bodies for references to other helpers
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		funcDef := pkgBuild.HelperFunctions[current]
		for name := range pkgBuild.HelperFunctions {
			if !needed[name] && strings.Contains(funcDef, name) {
				needed[name] = true
				queue = append(queue, name)
			}
		}
	}

	if len(needed) == 0 {
		return ""
	}

	var result strings.Builder

	funcKeys := make([]string, 0, len(needed))
	for k := range needed {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	for _, k := range funcKeys {
		result.WriteString(pkgBuild.HelperFunctions[k])
		result.WriteByte('\n')
	}

	return result.String()
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
func (pkgBuild *PKGBUILD) mapArrays(key string, data any) {
	arrData, ok := data.([]string)
	if !ok {
		return
	}

	arrayFields := map[string]*[]string{
		"arch":        &pkgBuild.Arch,
		"backup":      &pkgBuild.Backup,
		"conflicts":   &pkgBuild.Conflicts,
		"copyright":   &pkgBuild.Copyright,
		"depends":     &pkgBuild.Depends,
		"license":     &pkgBuild.License,
		"makedepends": &pkgBuild.MakeDepends,
		"optdepends":  &pkgBuild.OptDepends,
		"options":     &pkgBuild.Options,
		"provides":    &pkgBuild.Provides,
		"replaces":    &pkgBuild.Replaces,
		"sha256sums":  &pkgBuild.HashSums,
		"sha512sums":  &pkgBuild.HashSums,
		"source":      &pkgBuild.SourceURI,
	}

	if field, exists := arrayFields[key]; exists {
		*field = arrData
	} else {
		pkgBuild.CustomArrays[key] = arrData
	}
}

// mapFunctions reads a function name and its content and maps them to the
// PKGBUILD struct.
func (pkgBuild *PKGBUILD) mapFunctions(key string, data any) {
	funcBody, ok := data.(FuncBody)
	if !ok {
		return
	}

	body := string(funcBody)

	switch key {
	case "build":
		pkgBuild.Build = os.ExpandEnv(body)
	case "package":
		pkgBuild.Package = os.ExpandEnv(body)
	case "preinst":
		pkgBuild.PreInst = body
	case "prepare":
		pkgBuild.Prepare = os.ExpandEnv(body)
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
		pkgBuild.HelperFunctions[key] = fmt.Sprintf("%s() {\n%s\n}", key, body)
	}
}

// mapVariables reads a variable name and its content and maps them to the
// PKGBUILD struct.
func (pkgBuild *PKGBUILD) mapVariables(key string, data any) {
	strData, ok := data.(string)
	if !ok {
		return
	}

	varFields := map[string]*string{
		"debconf_config":   &pkgBuild.DebConfig,
		"debconf_template": &pkgBuild.DebTemplate,
		"epoch":            &pkgBuild.Epoch,
		"install":          &pkgBuild.Install,
		"maintainer":       &pkgBuild.Maintainer,
		"pkgdesc":          &pkgBuild.PkgDesc,
		"pkgname":          &pkgBuild.PkgName,
		"pkgrel":           &pkgBuild.PkgRel,
		"pkgver":           &pkgBuild.PkgVer,
		"priority":         &pkgBuild.Priority,
		"section":          &pkgBuild.Section,
		"url":              &pkgBuild.URL,
	}

	// Keys that require corresponding environment variables.
	envKeys := map[string]bool{
		"epoch": true, "maintainer": true, "pkgname": true,
		"pkgrel": true, "pkgver": true, "url": true,
	}

	field, isKnown := varFields[key]
	if isKnown {
		*field = strData
	} else {
		pkgBuild.CustomVariables[key] = strData
	}

	// Set environment variable if needed (known env keys + all custom variables).
	if envKeys[key] || !isKnown {
		err := os.Setenv(key, strData)
		if err != nil {
			osutils.Logger.Fatal("failed to set variable",
				osutils.Logger.Args("variable", key))
		}
	}
}

// parseDirective is a function that takes an input string and returns a key,
// priority, and an error.
func (pkgBuild *PKGBUILD) parseDirective(input string) (string, int, error) {
	// Split the input string by "__" to separate the key and the directive.
	split := strings.Split(input, "__")
	key := split[0]

	// If there is only one element in the split array, return the key and a priority of 0.
	if len(split) == 1 {
		return key, 0, nil
	}

	// If there are more than two elements in the split array, return an error.
	if len(split) != 2 {
		return key, 0, errors.Errorf("invalid use of '__' directive in %s", input)
	}

	// If the FullDistroName is empty, return the key and a priority of 0.
	if pkgBuild.FullDistroName == "" {
		return key, 0, nil
	}

	// Set the directive to the second element in the split array.
	directive := split[1]
	priority := -1

	// Check if the directive matches the FullDistroName and set the priority accordingly.
	if directive == pkgBuild.FullDistroName {
		priority = 3
	} else if constants.DistrosSet.Contains(directive) &&
		directive == pkgBuild.Distro {
		priority = 2
	} else if constants.PackagersSet.Contains(directive) &&
		directive == constants.DistroPackageManager[pkgBuild.Distro] {
		priority = 1
	}

	// Return the key, priority, and no error.
	return key, priority, nil
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
