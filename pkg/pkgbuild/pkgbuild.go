package pkgbuild

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/github/go-spdx/v2/spdxexp"
	"github.com/pkg/errors"
)

// Verbose is a flag to enable verbose output.
var Verbose bool

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions). It adds some exotics fields to manage debconfig
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch           []string
	ArchComputed   string
	Backup         []string
	Build          string
	Codename       string
	Conflicts      []string
	Copyright      []string
	DebConfig      string
	DebTemplate    string
	Depends        []string
	Distro         string
	Epoch          string
	Files          []string
	FullDistroName string
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
	PkgVer         string
	PostInst       string
	PostRm         string
	PreInst        string
	Prepare        string
	PreRelease     string
	PreRm          string
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

	if Verbose {
		if err := tmpl.Execute(utils.MultiPrinter.Writer, pkgBuild); err != nil {
			return err
		}
	}

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

	return utils.Exec(false, "", packageManager, args...)
}

// GetUpdates reads the package manager name and its arguments to perform
// a sync with remotes and consequently retrieve updates.
// It returns any error if encountered.
func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	return utils.Exec(false, "", packageManager, args...)
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

// ValidateGeneral checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateGeneral() {
	var checkErrors []string

	// Validate license
	if !pkgBuild.checkLicense() {
		checkErrors = append(checkErrors, "license")

		utils.Logger.Error("invalid SPDX license identifier",
			utils.Logger.Args("pkgname", pkgBuild.PkgName))
		utils.Logger.Info("you can find valid SPDX license identifiers here",
			utils.Logger.Args("🌐", "https://spdx.org/licenses/"))
	}

	// Check source and hash sums
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		checkErrors = append(checkErrors, "source-hash mismatch")

		utils.Logger.Error("number of sources and hashsums differs",
			utils.Logger.Args("pkgname", pkgBuild.PkgName))
	}

	// Check for package() function
	if pkgBuild.Package == "" {
		checkErrors = append(checkErrors, "package function")

		utils.Logger.Error("missing package() function",
			utils.Logger.Args("pkgname", pkgBuild.PkgName))
	}

	// Exit if there are validation errors
	if len(checkErrors) > 0 {
		os.Exit(1)
	}
}

// ComputeArchitecture checks if the specified architecture is supported.
// If "any", sets to "any". Otherwise, checks if current architecture is supported.
// Logs error if not supported, then sets to current architecture if supported.
func (pkgBuild *PKGBUILD) ComputeArchitecture() {
	isSupported := utils.Contains(pkgBuild.Arch, "any")
	if isSupported {
		pkgBuild.ArchComputed = "any"

		return
	}

	currentArch := utils.GetArchitecture()

	isSupported = utils.Contains(pkgBuild.Arch, currentArch)
	if !isSupported {
		utils.Logger.Fatal("unsupported architecture",
			utils.Logger.Args(
				"pkgname", pkgBuild.PkgName,
				"arch", strings.Join(pkgBuild.Arch, " ")))
	}

	pkgBuild.ArchComputed = currentArch
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
func (pkgBuild *PKGBUILD) mapArrays(key string, data any) {
	switch key {
	case "arch":
		pkgBuild.Arch = data.([]string)
	case "copyright":
		pkgBuild.Copyright = data.([]string)
	case "license":
		pkgBuild.License = data.([]string)
	case "depends":
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
	case "sha256sums":
		pkgBuild.HashSums = data.([]string)
	case "sha512sums":
		pkgBuild.HashSums = data.([]string)
	case "backup":
		pkgBuild.Backup = data.([]string)
	}
}

// mapFunctions reads a function name and its content and maps them to the
// PKGBUILD struct.
func (pkgBuild *PKGBUILD) mapFunctions(key string, data any) {
	switch key {
	case "build":
		pkgBuild.Build = os.ExpandEnv(data.(string))
	case "package":
		pkgBuild.Package = os.ExpandEnv(data.(string))
	case "preinst":
		pkgBuild.PreInst = data.(string)
	case "prepare":
		pkgBuild.Prepare = os.ExpandEnv(data.(string))
	case "postinst":
		pkgBuild.PostInst = data.(string)
	case "prerm":
		pkgBuild.PreRm = data.(string)
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
		utils.Logger.Fatal("failed to set variable",
			utils.Logger.Args("variable", key))
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

// setMainFolders sets the main folders for the PKGBUILD.
//
// It takes no parameters.
// It does not return anything.
func (pkgBuild *PKGBUILD) SetMainFolders() {
	switch pkgBuild.Distro {
	case "arch":
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "pkg", pkgBuild.PkgName)
	case "alpine":
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", pkgBuild.PkgName)
	default:
		pkgBuild.PackageDir, _ = os.MkdirTemp(
			pkgBuild.StartDir,
			pkgBuild.Distro)
	}

	if err := os.Setenv("pkgdir", pkgBuild.PackageDir); err != nil {
		utils.Logger.Fatal("failed to set variable pkgdir")
	}

	if err := os.Setenv("srcdir", pkgBuild.SourceDir); err != nil {
		utils.Logger.Fatal("failed to set variable srcdir")
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

// ValidateMandatoryItems checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateMandatoryItems() {
	var validationErrors []string

	// Check mandatory variables
	mandatoryChecks := map[string]string{
		"maintainer": pkgBuild.Maintainer,
		"pkgdesc":    pkgBuild.PkgDesc,
		"pkgname":    pkgBuild.PkgName,
		"pkgrel":     pkgBuild.PkgRel,
		"pkgver":     pkgBuild.PkgVer,
	}

	for variable, value := range mandatoryChecks {
		if value == "" {
			validationErrors = append(validationErrors, variable)
		}
	}

	// Exit if there are validation errors
	if len(validationErrors) > 0 {
		utils.Logger.Fatal(
			"failed to set variables",
			utils.Logger.Args(
				"variables",
				strings.Join(validationErrors, " ")))
	}
}
