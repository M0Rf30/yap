package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/builder"
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/packer"
	"github.com/M0Rf30/yap/pkg/parser"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/go-playground/validator/v10"
)

var (
	NoCache                      bool
	SkipSyncFlag                 bool
	SkipSyncBuildEnvironmentDeps bool
	UntilPkgName                 string
)

type DistroProject interface {
	Create() error
	Prepare() error
}

// MultipleProject defnes the content of yap.json specfile and some in-memory
// objects.
type MultipleProject struct {
	makeDepends    []string
	packageManager packer.Packer
	root           string
	BuildDir       string     `json:"buildDir"    validate:"required"`
	Description    string     `json:"description" validate:"required"`
	Name           string     `json:"name"        validate:"required"`
	Output         string     `json:"output"      validate:"required"`
	Projects       []*Project `json:"projects"    validate:"required,dive,required"`
}

type Project struct {
	Builder        *builder.Builder
	BuildRoot      string
	Distro         string
	MirrorRoot     string
	PackageManager packer.Packer
	Path           string
	Release        string
	Root           string
	Name           string `json:"name"    validate:"required,startsnotwith=.,startsnotwith=./"`
	HasToInstall   bool   `json:"install" validate:""`
}

func (mpc *MultipleProject) BuildAll() error {
	if UntilPkgName != "" {
		mpc.findPackageInProjects()
	}

	for _, proj := range mpc.Projects {
		fmt.Printf("%s🚀 :: %sLaunching build for package: %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			proj.Builder.PKGBUILD.PkgName,
		)

		if err := proj.Builder.Compile(); err != nil {
			return err
		}

		err := mpc.createPackages(proj)
		if err != nil {
			return err
		}

		if proj.HasToInstall {
			fmt.Printf("%s🤓 :: %s%s: installing package ...%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				proj.Name,
				string(constants.ColorWhite))

			if err := proj.PackageManager.Install(mpc.Output); err != nil {
				return err
			}
		}

		if UntilPkgName != "" && proj.Builder.PKGBUILD.PkgName == UntilPkgName {
			return nil
		}
	}

	return nil
}

func (mpc *MultipleProject) Clean() error {
	var err error

	for _, project := range mpc.Projects {
		err = utils.RemoveAll(project.Builder.PKGBUILD.PackageDir)
		if err != nil {
			return err
		}
	}

	if NoCache {
		for _, project := range mpc.Projects {
			err = utils.RemoveAll(project.Builder.PKGBUILD.SourceDir)
			if err != nil {
				return err
			}
		}
	}

	return err
}

func (mpc *MultipleProject) MultiProject(distro, release, path string) error {
	err := mpc.readProject(path)
	if err != nil {
		return err
	}

	err = mpc.validateAllProject(distro, release, path)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	mpc.packageManager = packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
	if !SkipSyncFlag {
		if err := mpc.packageManager.Update(); err != nil {
			return err
		}
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	if !SkipSyncBuildEnvironmentDeps {
		mpc.getMakeDeps()

		if err := mpc.packageManager.Prepare(mpc.makeDepends); err != nil {
			return err
		}
	}

	mpc.root = path

	return err
}

func (mpc *MultipleProject) createPackages(proj *Project) error {
	if mpc.Output != "" {
		mpc.Output, _ = filepath.Abs(mpc.Output)
	}

	if err := utils.ExistsMakeDir(mpc.Output); err != nil {
		return err
	}

	err := proj.PackageManager.Build(mpc.Output)
	if err != nil {
		return err
	}

	return err
}

func (mpc *MultipleProject) findPackageInProjects() {
	var matchFound bool

	for _, proj := range mpc.Projects {
		if UntilPkgName == proj.Builder.PKGBUILD.PkgName {
			matchFound = true
		}
	}

	if !matchFound {
		fmt.Printf("%s❌ :: %sPackage not found: %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			UntilPkgName,
		)

		os.Exit(1)
	}
}

func (mpc *MultipleProject) getMakeDeps() {
	var makeDepends []string

	for _, child := range mpc.Projects {
		makeDepends = append(makeDepends, child.Builder.PKGBUILD.MakeDepends...)
	}

	mpc.makeDepends = makeDepends
}

func (mpc *MultipleProject) populateProjects(distro, release, path string) error {
	var err error

	var projects = make([]*Project, 0)

	for _, child := range mpc.Projects {
		pkgbuildFile, err := parser.ParseFile(distro,
			release,
			filepath.Join(mpc.BuildDir, child.Name),
			filepath.Join(path, child.Name))
		if err != nil {
			return err
		}

		mpc.packageManager = packer.GetPackageManager(pkgbuildFile, distro)

		proj := &Project{
			Name:           child.Name,
			Builder:        &builder.Builder{PKGBUILD: pkgbuildFile},
			PackageManager: mpc.packageManager,
			HasToInstall:   child.HasToInstall,
		}

		projects = append(projects, proj)
	}

	mpc.Projects = projects

	return err
}

func (mpc *MultipleProject) readProject(path string) error {
	cleanFilePath := filepath.Clean(filepath.Join(path, "yap.json"))

	filePath, err := os.Open(cleanFilePath)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open yap.json file within '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			cleanFilePath,
			string(constants.ColorWhite))
		os.Exit(1)
	}

	prjContent, err := io.ReadAll(filePath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(prjContent, &mpc)
	if err != nil {
		return err
	}

	err = mpc.validateJSON()
	if err != nil {
		return err
	}

	return err
}

func (mpc *MultipleProject) validateAllProject(distro, release, path string) error {
	var err error
	for _, child := range mpc.Projects {
		pkgbuildFile, err := parser.ParseFile(distro,
			release,
			filepath.Join(mpc.BuildDir, child.Name),
			filepath.Join(path, child.Name))
		if err != nil {
			return err
		}

		pkgbuildFile.Validate()
	}

	return err
}

func (mpc *MultipleProject) validateJSON() error {
	validate := validator.New()

	err := validate.Struct(mpc)

	if err != nil {
		return err
	}

	return err
}
