package pkgbuild

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/utils"
	"mvdan.cc/sh/v3/shell"
)

// Verbose is a flag to enable verbose output.
var Verbose bool

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions). It adds some exotics fields to manage debconfig
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch           []string
	Backup         []string
	Build          string
	Conflicts      []string
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
	PreRelease     string
	PreRm          string
	Prepare        string
	Priority       string
	Provides       []string
	Codename       string
	StartDir       string
	Section        string
	SourceDir      string
	SourceURI      []string
	URL            string
	priorities     map[string]int
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
	pkgBuild.setMainFolders()
	pkgBuild.mapArrays(key, data)
	pkgBuild.mapFunctions(key, data)

	return nil
}

// CreateSpec reads the filepath where the specfile will be written and the
// content of the specfile. Specfile generation is done using go templates for
// every different distro family. It returns any error if encountered.
func (pkgBuild *PKGBUILD) CreateSpec(filePath, script string) error {
	cleanFilePath := filepath.Clean(filePath)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

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

	if Verbose {
		err = tmpl.Execute(os.Stdout, pkgBuild)
		if err != nil {
			return err
		}
	}

	return tmpl.Execute(file, pkgBuild)
}

// GetDepends reads the package manager name, its arguments and all the
// dependencies required to build the package. It returns any error if
// encountered.
func (pkgBuild *PKGBUILD) GetDepends(packageManager string, args, makeDepends []string) error {
	var err error
	if len(makeDepends) == 0 {
		return err
	}

	args = append(args, makeDepends...)

	err = utils.Exec("", packageManager, args...)
	if err != nil {
		return err
	}

	return nil
}

// GetUpdates reads the package manager name and its arguments to perform
// a sync with remotes and consequently retrieve updates.
// It returns any error if encountered.
func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	err := utils.Exec("", packageManager, args...)
	if err != nil {
		return err
	}

	return nil
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

// Validate checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) Validate() {
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		log.Fatalf("%s%s ❌ :: %snumber of sources and hashsums differs%s\n",
			pkgBuild.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
	}

	if pkgBuild.Package == "" {
		log.Fatalf("%s%s ❌ :: %smissing package() function%s\n",
			pkgBuild.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
	}
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
func (pkgBuild *PKGBUILD) mapArrays(key string, data any) {
	switch key {
	case "arch":
		pkgBuild.Arch = data.([]string)
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
		pkgBuild.Build, _ = shell.Expand(data.(string), os.Getenv)
	case "package":
		pkgBuild.Package, _ = shell.Expand(data.(string), os.Getenv)
	case "preinst":
		pkgBuild.PreInst = data.(string)
	case "prepare":
		pkgBuild.Prepare, _ = shell.Expand(data.(string), os.Getenv)
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
		log.Fatalf("%s❌ :: %sfailed to set variable %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), key)
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
		return key, 0, fmt.Errorf("pack: Invalid use of '__' directive in '%w'", nil)
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
func (pkgBuild *PKGBUILD) setMainFolders() {
	switch pkgBuild.Distro {
	case "arch":
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "pkg", pkgBuild.PkgName)
	case "alpine":
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", pkgBuild.PkgName)
	}

	if err := os.Setenv("pkgdir", pkgBuild.PackageDir); err != nil {
		log.Fatalf("%s❌ :: %sfailed to set variable pkgdir\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))
	}

	if err := os.Setenv("srcdir", pkgBuild.SourceDir); err != nil {
		log.Fatalf("%s❌ :: %sfailed to set variable srcdir\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))
	}
}
