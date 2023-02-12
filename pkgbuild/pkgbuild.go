package pkgbuild

import (
	"fmt"
	"os"
	"strings"

	"github.com/M0Rf30/yap/constants"
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
	FullDistroName string
	Functions      map[string]string
	HashSums       []string
	Home           string
	Install        string
	License        []string
	Maintainer     string
	MakeDepends    []string
	OptDepends     []string
	Options        []string
	Package        string
	PackageDir     string
	PkgDesc        string
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
	CodeName       string
	Root           string
	Section        string
	SourceDir      string
	Source         []string
	URL            string
	Variables      map[string]string
	priorities     map[string]int
}

func (p *PKGBUILD) Init() {
	p.priorities = map[string]int{}

	p.FullDistroName = p.Distro
	if p.CodeName != "" {
		p.FullDistroName += "-" + p.CodeName
	}
}

func (p *PKGBUILD) checkDistroAssignments() int {
	var priority int

	if constants.ReleasesSet.Contains(p.CodeName) {
		if p.CodeName == p.FullDistroName {
			priority = 3
		}

		return priority
	}

	if constants.DistrosSet.Contains(p.CodeName) {
		if p.CodeName == p.Distro {
			priority = 2
		}

		return priority
	}

	if constants.PackagersSet.Contains(p.CodeName) {
		if p.CodeName == constants.DistroPackageManager[p.Distro] {
			priority = 1
		}

		return priority
	}

	return priority
}

func (p *PKGBUILD) lookForDistroAssignments(input string) (string, int, error) {
	splittedKey := strings.Split(input, "__")
	key := splittedKey[0]

	var err error

	var priority int

	switch {
	case len(splittedKey) == 1:
		priority = 0

		return key, priority, err
	case len(splittedKey) != 2:
		return key, priority, fmt.Errorf("pack: Invalid use of '__' directive in '%w'", err)
	default:
		priority = -1
	}

	if p.Distro == "" {
		return key, priority, err
	}

	return key, priority, err
}

func (p *PKGBUILD) checkAssignment(key string) (string, int, error) {
	key, priority, err := p.lookForDistroAssignments(key)
	if err != nil {
		return key, priority, err
	}

	priority = p.checkDistroAssignments()

	if key == "pkgver" || key == "pkgrel" {
		return key, priority, fmt.Errorf("pack: Cannot use directive for '%w'", err)
	}

	if priority == -1 {
		return key, priority, err
	}

	if priority < p.priorities[key] {
		return key, priority, err
	}

	return key, priority, err
}

func (p *PKGBUILD) AddCustomAssignment(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

	switch key {
	case "maintainer":
		p.Maintainer = data.(string)
	case "section":
		p.Section = data.(string)
	case "priority":
		p.Priority = data.(string)
	case "debconf_template":
		p.DebTemplate = data.(string)
	case "debconf_config":
		p.DebConfig = data.(string)
	default:
		return err
	}

	if p.Variables != nil {
		p.Variables[key] = data.(string)
	} else {
		return err
	}

	if p.Functions != nil {
		p.Functions[key] = data.(string)
	} else {
		return err
	}

	return err
}

func (p *PKGBUILD) AddDependencies(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

	switch key {
	case "depends":
		p.Depends = data.([]string)
	case "optdepends":
		p.OptDepends = data.([]string)
	case "makedepends":
		p.MakeDepends = data.([]string)
	case "provides":
		p.Provides = data.([]string)
	case "conflicts":
		p.Conflicts = data.([]string)
	default:
		return err
	}

	return err
}

func (p *PKGBUILD) AddSources(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

	switch key {
	case "source":
		p.Source = data.([]string)
	case "sha256sums":
		p.HashSums = data.([]string)
	case "sha512sums":
		p.HashSums = data.([]string)
	default:
		return err
	}

	return err
}

func (p *PKGBUILD) AddFunctions(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

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

	return err
}

func (p *PKGBUILD) AddNonMandatoryItem(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

	switch key {
	case "install":
		p.Install = data.(string)
	case "options":
		p.Options = data.([]string)
	case "backup":
		p.Backup = data.([]string)
	default:
		return err
	}

	return err
}

func (p *PKGBUILD) AddItem(key string, data interface{}) error {
	key, priority, err := p.checkAssignment(key)

	p.priorities[key] = priority

	switch key {
	case "pkgname":
		p.PkgName = data.(string)
	case "pkgver":
		p.PkgVer = data.(string)
	case "pkgrel":
		p.PkgRel = data.(string)
	case "pkgdesc":
		p.PkgDesc = data.(string)
	case "arch":
		p.Arch = data.([]string)
	case "license":
		p.License = data.([]string)
	case "url":
		p.URL = data.(string)
	default:
		return err
	}

	return err
}

func (p *PKGBUILD) Validate() {
	if len(p.Source) != len(p.HashSums) {
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
