// Package pkgbuild provides PKGBUILD structure and manipulation functionality.
package pkgbuild

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/github/go-spdx/v2/spdxexp"
	"github.com/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

const (
	dependsKey = "depends"
)

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions).
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch           []string
	ArchComputed   string
	Backup         []string
	Build          string
	BuildDate      int64
	Checksum       string
	Codename       string
	Conflicts      []string
	Copyright      []string
	DataHash       string
	DebConfig      string
	DebTemplate    string
	Depends        []string
	Distro         string
	Epoch          string
	Files          []string
	FullDistroName string
	Group          string
	HashSums       []string
	Home           string
	Install        string
	InstalledSize  int64
	License        []string
	Maintainer     string
	MakeDepends    []string
	OptDepends     []string
	Options        []string
	Package        string
	PackageDir     string
	PkgDesc        string
	PkgDest        string
	PkgName        string
	PkgRel         string
	PkgType        string
	PkgVer         string
	PostInst       string
	PostRm         string
	PostTrans      string
	PreInst        string
	Prepare        string
	PreRelease     string
	PreRm          string
	PreTrans       string
	priorities     map[string]int
	Priority       string
	Provides       []string
	Replaces       []string
	Section        string
	SourceDir      string
	SourceURI      []string
	StartDir       string
	URL            string
	StaticEnabled  bool
	StripEnabled   bool
	YAPVersion     string
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
		logger.Fatal("unsupported architecture",
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
			logger.Warn("failed to close pkgbuild file",
				"path", cleanFilePath,
				"error", err)
		}
	}()

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

	pkgBuild.FullDistroName = pkgBuild.Distro
	if pkgBuild.Codename != "" {
		pkgBuild.FullDistroName += "_" + pkgBuild.Codename
	}
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
		key := make([]byte, 5)

		_, err := rand.Read(key)
		if err != nil {
			logger.Fatal("fatal error", "error", err)
		}

		randomString := hex.EncodeToString(key)
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, pkgBuild.Distro+"-"+randomString)
	}

	err := os.Setenv("pkgdir", pkgBuild.PackageDir)
	if err != nil {
		logger.Fatal("failed to set variable pkgdir")
	}

	err = os.Setenv("srcdir", pkgBuild.SourceDir)
	if err != nil {
		logger.Fatal("failed to set variable srcdir")
	}

	err = os.Setenv("startdir", pkgBuild.StartDir)
	if err != nil {
		logger.Fatal("failed to set variable startdir")
	}
}

// ValidateGeneral checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateGeneral() {
	var checkErrors []string

	// Validate license
	if !pkgBuild.checkLicense() {
		checkErrors = append(checkErrors, "license")

		logger.Error("invalid SPDX license identifier",
			"pkgname", pkgBuild.PkgName)
		logger.Info("you can find valid SPDX license identifiers here",
			"🌐", "https://spdx.org/licenses/")
	}

	// Check source and hash sums
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		checkErrors = append(checkErrors, "source-hash mismatch")

		logger.Error("number of sources and hashsums differs",
			"pkgname", pkgBuild.PkgName)
	}

	// Check for package() function
	if pkgBuild.Package == "" {
		checkErrors = append(checkErrors, "package function")

		logger.Error("missing package() function",
			"pkgname", pkgBuild.PkgName)
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
		logger.Fatal(
			"failed to set variables",
			"variables",
			strings.Join(validationErrors, " "))
	}
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
func (pkgBuild *PKGBUILD) mapArrays(key string, data any) {
	if pkgBuild.mapChecksumsArrays(key, data) {
		return
	}

	switch key {
	case "arch":
		pkgBuild.Arch = data.([]string)
	case "copyright":
		pkgBuild.Copyright = data.([]string)
	case "license":
		pkgBuild.License = data.([]string)
	case dependsKey:
		pkgBuild.Depends = data.([]string)
	case "options":
		pkgBuild.Options = data.([]string)
	case "optdepends":
		pkgBuild.OptDepends = data.([]string)
	case "makedepends":
		pkgBuild.MakeDepends = data.([]string)
	case "provides":
		pkgBuild.Provides = data.([]string)
	case "conflicts":
		pkgBuild.Conflicts = data.([]string)
	case "replaces":
		pkgBuild.Replaces = data.([]string)
	case "source":
		pkgBuild.SourceURI = data.([]string)
	case "backup":
		pkgBuild.Backup = data.([]string)
	}
}

// mapChecksumsArrays handles mapping of checksum arrays and returns true if handled
func (pkgBuild *PKGBUILD) mapChecksumsArrays(key string, data any) bool {
	switch key {
	case "sha512sums":
		pkgBuild.HashSums = data.([]string)
		return true
	case "sha384sums":
		pkgBuild.HashSums = data.([]string)
		return true
	case "sha256sums":
		pkgBuild.HashSums = data.([]string)
		return true
	case "sha224sums":
		pkgBuild.HashSums = data.([]string)
		return true
	case "b2sums":
		pkgBuild.HashSums = data.([]string)
		return true
	case "cksums":
		pkgBuild.HashSums = data.([]string)
		return true
	default:
		return false
	}
}

// mapFunctions reads a function name and its content and maps them to the
// PKGBUILD struct.
func (pkgBuild *PKGBUILD) mapFunctions(key string, data any) {
	switch key {
	case "build":
		// Don't use os.ExpandEnv here as it removes runtime variables like ${bin}
		// Variable expansion is now handled properly in the parser
		pkgBuild.Build = data.(string)
	case "package":
		// Don't use os.ExpandEnv here as it removes runtime variables
		// Variable expansion is now handled properly in the parser
		pkgBuild.Package = data.(string)
	case "preinst":
		pkgBuild.PreInst = data.(string)
	case "prepare":
		// Don't use os.ExpandEnv here as it removes runtime variables
		// Variable expansion is now handled properly in the parser
		pkgBuild.Prepare = data.(string)
	case "postinst":
		pkgBuild.PostInst = data.(string)
	case "posttrans":
		pkgBuild.PostTrans = data.(string)
	case "prerm":
		pkgBuild.PreRm = data.(string)
	case "pretrans":
		pkgBuild.PreTrans = data.(string)
	case "postrm":
		pkgBuild.PostRm = data.(string)
	}
}

// mapVariables reads a variable name and its content and maps them to the
// PKGBUILD struct.
func (pkgBuild *PKGBUILD) mapVariables(key string, data any) {
	var err error

	switch key {
	case "pkgname":
		err = os.Setenv(key, data.(string))
		pkgBuild.PkgName = data.(string)
	case "epoch":
		err = os.Setenv(key, data.(string))
		pkgBuild.Epoch = data.(string)
	case "pkgver":
		err = os.Setenv(key, data.(string))
		pkgBuild.PkgVer = data.(string)
	case "pkgrel":
		err = os.Setenv(key, data.(string))
		pkgBuild.PkgRel = data.(string)
	case "pkgdesc":
		pkgBuild.PkgDesc = data.(string)
	case "maintainer":
		err = os.Setenv(key, data.(string))
		pkgBuild.Maintainer = data.(string)
	case "section":
		pkgBuild.Section = data.(string)
	case "priority":
		pkgBuild.Priority = data.(string)
	case "url":
		err = os.Setenv(key, data.(string))
		pkgBuild.URL = data.(string)
	case "debconf_template":
		pkgBuild.DebTemplate = data.(string)
	case "debconf_config":
		pkgBuild.DebConfig = data.(string)
	case "install":
		pkgBuild.Install = data.(string)
	}

	if err != nil {
		logger.Fatal("failed to set variable",
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

	currentArch := osutils.GetArchitecture()
	if possibleArch != currentArch {
		return key, -1, true // Invalid architecture for current system
	}

	// Check distribution part
	distPriority := pkgBuild.getDistributionPriority(distributionPart)
	if distPriority > 0 {
		return key, distPriority + 4, true // Add 4 to boost arch+distro combinations
	}

	return key, -1, true
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

	currentArch := osutils.GetArchitecture()
	if possibleArch == currentArch {
		return key, 4, true // Higher priority than distribution-specific
	}

	return key, -1, true // Invalid architecture for current system
}

// parseDistributionOnly handles distribution-only syntax
func (pkgBuild *PKGBUILD) parseDistributionOnly(input string) (
	key string, priority int, err error,
) {
	split := strings.Split(input, "__")
	key = split[0]

	if len(split) == 1 {
		return key, 0, nil
	}

	if len(split) != 2 {
		return key, 0, errors.Errorf("invalid use of '__' directive in %s", input)
	}

	if pkgBuild.FullDistroName == "" {
		return key, 0, nil
	}

	directive := split[1]
	priority = pkgBuild.getDistributionPriority(directive)

	return key, priority, nil
}

// getDistributionPriority returns the priority for a distribution directive
func (pkgBuild *PKGBUILD) getDistributionPriority(directive string) int {
	switch {
	case directive == pkgBuild.FullDistroName:
		return 3
	case constants.DistrosSet.Contains(directive) &&
		directive == pkgBuild.Distro:
		return 2
	case constants.PackagersSet.Contains(directive) &&
		directive == constants.DistroPackageManager[pkgBuild.Distro]:
		return 1
	default:
		return -1
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
		"any",      // Architecture-independent packages
	}

	return osutils.Contains(validArchitectures, arch)
}
