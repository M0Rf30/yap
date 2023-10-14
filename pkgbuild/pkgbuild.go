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

func (p *PKGBUILD) mapArrays(key string, data interface{}) {
	switch key {
	case "arch":
		p.Arch = data.([]string)
	case "license":
		p.License = data.([]string)
	case "depends":
		p.Depends = data.([]string)
	case "options":
		p.Options = data.([]string)
	case "optdepends":
		p.OptDepends = data.([]string)
	case "makedepends":
		p.MakeDepends = data.([]string)
	case "provides":
		p.Provides = data.([]string)
	case "conflicts":
		p.Conflicts = data.([]string)
	case "source":
		p.SourceURI = data.([]string)
	case "sha256sums":
		p.HashSums = data.([]string)
	case "sha512sums":
		p.HashSums = data.([]string)
	case "backup":
		p.Backup = data.([]string)
	default:
	}
}

func (p *PKGBUILD) mapFunctions(key string, data interface{}) {
	switch key {
	case "build":
		p.Build = data.(string)
	case "package":
		p.Package = data.(string)
	case "preinst":
		p.PreInst = data.(string)
	case "prepare":
		p.Prepare = data.(string)
	case "postinst":
		p.PostInst = data.(string)
	case "prerm":
		p.PreRm = data.(string)
	case "postrm":
		p.PostRm = data.(string)
	default:
	}
}

func (p *PKGBUILD) mapVariables(key string, data interface{}) {
	switch key {
	case "pkgname":
		p.PkgName = data.(string)
	case "epoch":
		p.Epoch = data.(string)
	case "pkgver":
		p.PkgVer = data.(string)
	case "pkgrel":
		p.PkgRel = data.(string)
	case "pkgdesc":
		p.PkgDesc = data.(string)
	case "maintainer":
		p.Maintainer = data.(string)
	case "section":
		p.Section = data.(string)
	case "priority":
		p.Priority = data.(string)
	case "url":
		p.URL = data.(string)
	case "debconf_template":
		p.DebTemplate = data.(string)
	case "debconf_config":
		p.DebConfig = data.(string)
	case "install":
		p.Install = data.(string)
	default:
	}
}

func (p *PKGBUILD) Init() {
	p.priorities = map[string]int{}

	p.FullDistroName = p.Distro
	if p.Codename != "" {
		p.FullDistroName += "_" + p.Codename
	}
}

func (p *PKGBUILD) parseDirective(input string) (string, int, error) {
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

	if p.FullDistroName == "" {
		return key, priority, err
	}

	if key == "pkgver" || key == "pkgrel" {
		return key, priority, fmt.Errorf("pack: Cannot use directive for '%w'", err)
	}

	directive := split[1]

	if directive == p.FullDistroName {
		priority = 3
	}

	if constants.DistrosSet.Contains(directive) {
		if directive == p.Distro {
			priority = 2
		}

		return key, priority, err
	}

	if constants.PackagersSet.Contains(directive) {
		if directive == constants.DistroPackageManager[p.Distro] {
			priority = 1
		}

		return key, priority, err
	}

	return key, priority, err
}

func (p *PKGBUILD) AddItem(key string, data interface{}) error {
	key, priority, err := p.parseDirective(key)
	if err != nil {
		return err
	}

	if priority == -1 {
		return err
	}

	if priority < p.priorities[key] {
		return err
	}

	p.priorities[key] = priority

	p.mapVariables(key, data)
	p.mapArrays(key, data)
	p.mapFunctions(key, data)

	return err
}

func (p *PKGBUILD) Validate() {
	if len(p.SourceURI) != len(p.HashSums) {
		fmt.Printf("%s%s ❌ :: %snumber of sources and hashsums differs%s\n",
			p.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
		os.Exit(1)
	}

	if len(p.Package) == 0 {
		fmt.Printf("%s%s ❌ :: %smissing package() function%s\n",
			p.PkgName,
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
		os.Exit(1)
	}
}

func (p *PKGBUILD) GetDepends(packageManager string, args []string, makeDepends []string) error {
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

func (p *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	err := utils.Exec("", packageManager, args...)
	if err != nil {
		return err
	}

	return err
}

func (p *PKGBUILD) CreateSpec(filePath string, script string) error {
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
		err = tmpl.Execute(os.Stdout, p)
		if err != nil {
			log.Panic(err)
		}
	}

	err = tmpl.Execute(writer, p)
	if err != nil {
		log.Panic(err)
	}

	return err
}
