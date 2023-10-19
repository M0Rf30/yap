package pkgbuild

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/utils"
	"mvdan.cc/sh/v3/shell"
)

var Verbose bool

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

func (pkgBuild *PKGBUILD) AddItem(key string, data any) error {
	key, priority, err := pkgBuild.parseDirective(key)
	if err != nil {
		return err
	}

	if priority == -1 {
		return err
	}

	if priority < pkgBuild.priorities[key] {
		return err
	}

	pkgBuild.priorities[key] = priority
	pkgBuild.mapVariables(key, data)
	pkgBuild.setMainFolders()
	pkgBuild.mapArrays(key, data)
	pkgBuild.mapFunctions(key, data)

	return err
}

func (pkgBuild *PKGBUILD) CreateSpec(filePath, script string) error {
	cleanFilePath := filepath.Clean(filePath)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		log.Panic(err)
	}

	defer file.Close()
	writer := io.Writer(file)

	tmpl := template.New("template")
	tmpl.Funcs(template.FuncMap{
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
			log.Panic(err)
		}
	}

	err = tmpl.Execute(writer, pkgBuild)
	if err != nil {
		log.Panic(err)
	}

	return err
}

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

	return err
}

func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	err := utils.Exec("", packageManager, args...)
	if err != nil {
		return err
	}

	return err
}

func (pkgBuild *PKGBUILD) Init() {
	pkgBuild.priorities = map[string]int{}

	pkgBuild.FullDistroName = pkgBuild.Distro
	if pkgBuild.Codename != "" {
		pkgBuild.FullDistroName += "_" + pkgBuild.Codename
	}
}

func (pkgBuild *PKGBUILD) Validate() {
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		fmt.Printf("%s%s ❌ :: %snumber of sources and hashsums differs%s\n",
			pkgBuild.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
		os.Exit(1)
	}

	if pkgBuild.Package == "" {
		fmt.Printf("%s%s ❌ :: %smissing package() function%s\n",
			pkgBuild.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
		os.Exit(1)
	}
}

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
		fmt.Printf("%s❌ :: %sfailed to set variable %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), key)

		os.Exit(1)
	}
}

func (pkgBuild *PKGBUILD) parseDirective(input string) (string, int, error) {
	split := strings.Split(input, "__")
	key := split[0]

	var err error

	var priority int

	numElem := 2

	switch {
	case len(split) == 1:
		priority = 0

		return key, priority, err
	case len(split) != numElem:
		return key, priority, fmt.Errorf("pack: Invalid use of '__' directive in '%w'", err)
	default:
		priority = -1
	}

	if pkgBuild.FullDistroName == "" {
		return key, priority, err
	}

	if key == "pkgver" || key == "pkgrel" {
		return key, priority, fmt.Errorf("pack: Cannot use directive for '%w'", err)
	}

	directive := split[1]

	if directive == pkgBuild.FullDistroName {
		priority = 3
	}

	if constants.DistrosSet.Contains(directive) {
		if directive == pkgBuild.Distro {
			priority = 2
		}

		return key, priority, err
	}

	if constants.PackagersSet.Contains(directive) {
		if directive == constants.DistroPackageManager[pkgBuild.Distro] {
			priority = 1
		}

		return key, priority, err
	}

	return key, priority, err
}

func (pkgBuild *PKGBUILD) setMainFolders() {
	if pkgBuild.Distro == "arch" {
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "pkg", pkgBuild.PkgName)
	}

	if pkgBuild.Distro == "alpine" {
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", pkgBuild.PkgName)
	}

	err := os.Setenv("pkgdir", pkgBuild.PackageDir)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to set variable pkgdir\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))

		os.Exit(1)
	}

	err = os.Setenv("srcdir", pkgBuild.SourceDir)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to set variable srcdir\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))

		os.Exit(1)
	}
}
