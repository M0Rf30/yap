package project

import (
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/builder"
	"github.com/M0Rf30/yap/pkg/packer"
	"github.com/M0Rf30/yap/pkg/parser"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
)

var (
	CleanBuild   bool
	NoBuild      bool
	NoMakeDeps   bool
	SkipSyncDeps bool
)

// FromPkgName is used to start the build process from a specific package.
// ToPkgName is used to stop the build process after a specific package.
var (
	FromPkgName string
	ToPkgName   string
)

// DistroProject is an interface that defines the methods for creating and
// preparing a project for a specific distribution.
//
// It includes the following methods:
//   - Create(): error
//     This method is responsible for creating the project.
//   - Prepare(): error
//     This method is responsible for preparing the project.
type DistroProject interface {
	Create() error
	Prepare() error
}

// MultipleProject represents a collection of projects.
//
// It contains a slice of Project objects and provides methods to interact
// with multiple projects. The methods include building all the projects,
// finding a package in the projects, and creating packages for each project.
// The MultipleProject struct also contains the output directory for the
// packages and the ToPkgName field, which can be used to stop the build
// process after a specific package.
type MultipleProject struct {
	makeDepends    []string
	packageManager packer.Packer
	root           string
	BuildDir       string     `json:"buildDir"    validate:"required"`
	Description    string     `json:"description" validate:"required"`
	Name           string     `json:"name"        validate:"required"`
	Output         string     `json:"output"      validate:"required"`
	Projects       []*Project `json:"projects"    validate:"required,dive,required"`
	singleProject  bool
}

// Project represents a single project.
//
// It contains the necessary information to build and manage the project.
// The fields include the project's name, the path to the project directory,
// the builder object, the package manager object, and a flag indicating
// whether the project has to be installed.
type Project struct {
	Builder        *builder.Builder
	BuildRoot      string
	Distro         string
	PackageManager packer.Packer
	Path           string
	Release        string
	Root           string
	Name           string `json:"name"    validate:"required,startsnotwith=.,startsnotwith=./"`
	HasToInstall   bool   `json:"install" validate:""`
}

// BuildAll builds all the projects in the MultipleProject struct.
//
// It compiles each project's package and creates the necessary packages.
// If the project has to be installed, it installs the package.
// If ToPkgName is not empty, it stops building after the specified package.
// It returns an error if any error occurs during the build process.
func (mpc *MultipleProject) BuildAll() error {
	if !mpc.singleProject {
		mpc.checkPkgsRange(FromPkgName, ToPkgName)
	}

	for _, proj := range mpc.Projects {
		if FromPkgName != "" && proj.Builder.PKGBUILD.PkgName != FromPkgName {
			continue
		}

		utils.Logger.Info("making package", utils.Logger.Args("name", proj.Builder.PKGBUILD.PkgName,
			"pkgver", proj.Builder.PKGBUILD.PkgVer,
			"pkgrel", proj.Builder.PKGBUILD.PkgRel))

		if err := proj.Builder.Compile(NoBuild); err != nil {
			return err
		}

		if !NoBuild {
			if err := mpc.createPackages(proj); err != nil {
				return err
			}
		}

		if proj.HasToInstall {
			utils.Logger.Info("installing package", utils.Logger.Args("pkgname", proj.Builder.PKGBUILD.PkgName,
				"pkgver", proj.Builder.PKGBUILD.PkgVer,
				"pkgrel", proj.Builder.PKGBUILD.PkgRel))

			if err := proj.PackageManager.Install(mpc.Output); err != nil {
				return err
			}
		}

		if ToPkgName != "" && proj.Builder.PKGBUILD.PkgName == ToPkgName {
			return nil
		}
	}

	return nil
}

// Clean cleans up the MultipleProject by removing the package directories and
// source directories if the NoCache flag is set. It takes no parameters. It
// returns an error if there was a problem removing the directories.
func (mpc *MultipleProject) Clean() error {
	for _, project := range mpc.Projects {
		if err := utils.RemoveAll(project.Builder.PKGBUILD.PackageDir); err != nil {
			return err
		}

		if CleanBuild {
			if err := utils.RemoveAll(project.Builder.PKGBUILD.SourceDir); err != nil {
				return err
			}
		}
	}

	return nil
}

// MultiProject is a function that performs multiple project operations.
//
// It takes in the following parameters:
// - distro: a string representing the distribution
// - release: a string representing the release
// - path: a string representing the path
//
// It returns an error.
func (mpc *MultipleProject) MultiProject(distro, release, path string) error {
	if err := mpc.readProject(path); err != nil {
		return err
	}

	err := mpc.validateAllProject(distro, release, path)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	mpc.packageManager = packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
	if !SkipSyncDeps {
		if err := mpc.packageManager.Update(); err != nil {
			return err
		}
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	if !NoMakeDeps {
		mpc.getMakeDeps()

		if err := mpc.packageManager.Prepare(mpc.makeDepends); err != nil {
			return err
		}
	}

	mpc.root = path

	return nil
}

// checkPkgsRange checks the range of packages from `fromPkgName` to `toPkgName`.
//
// It takes two parameters:
// - fromPkgName: string representing the name of the starting package.
// - toPkgName: string representing the name of the ending package.
func (mpc *MultipleProject) checkPkgsRange(fromPkgName, toPkgName string) {
	var firstIndex, lastIndex int

	if fromPkgName != "" {
		firstIndex = mpc.findPackageInProjects(fromPkgName)
	}

	if toPkgName != "" {
		lastIndex = mpc.findPackageInProjects(toPkgName)
	}

	if fromPkgName != "" && toPkgName != "" && firstIndex > lastIndex {
		utils.Logger.Fatal("invalid package order: %s should be built before %s",
			utils.Logger.Args(fromPkgName, toPkgName))
	}
}

// createPackages creates packages for the MultipleProject.
//
// It takes a pointer to a MultipleProject as a receiver and a pointer to a Project as a parameter.
// It returns an error.
func (mpc *MultipleProject) createPackages(proj *Project) error {
	if mpc.Output != "" {
		absOutput, err := filepath.Abs(mpc.Output)
		if err != nil {
			return err
		}

		mpc.Output = absOutput
	}

	if err := utils.ExistsMakeDir(mpc.Output); err != nil {
		return err
	}

	if err := proj.PackageManager.Build(mpc.Output); err != nil {
		return err
	}

	return nil
}

// findPackageInProjects finds a package in the MultipleProject struct.
//
// pkgName: the name of the package to find.
// int: the index of the package if found, else -1.
func (mpc *MultipleProject) findPackageInProjects(pkgName string) int {
	var matchFound bool

	var index int

	for i, proj := range mpc.Projects {
		if pkgName == proj.Builder.PKGBUILD.PkgName {
			matchFound = true
			index = i
		}
	}

	if !matchFound {
		utils.Logger.Fatal("package not found",
			utils.Logger.Args("pkgname", pkgName))
	}

	return index
}

// getMakeDeps retrieves the make dependencies for the MultipleProject.
//
// It iterates over each child project and appends their make dependencies
// to the makeDepends slice. The makeDepends slice is then assigned to the
// makeDepends field of the MultipleProject.
func (mpc *MultipleProject) getMakeDeps() {
	var makeDepends []string

	for _, child := range mpc.Projects {
		makeDepends = append(makeDepends, child.Builder.PKGBUILD.MakeDepends...)
	}

	mpc.makeDepends = makeDepends
}

// populateProjects populates the MultipleProject with projects based on the
// given distro, release, and path.
//
// distro: The distribution of the projects.
// release: The release version of the projects.
// path: The path to the projects.
// error: An error if any occurred during the population process.
func (mpc *MultipleProject) populateProjects(distro, release, path string) error {
	var projects = make([]*Project, 0)

	for _, child := range mpc.Projects {
		startDir := filepath.Join(mpc.BuildDir, child.Name)
		home := filepath.Join(path, child.Name)

		pkgbuildFile, err := parser.ParseFile(distro,
			release,
			startDir,
			home)
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

	return nil
}

// readProject reads the project file at the specified path and populates the MultipleProject struct.
//
// It takes a string parameter `path` which represents the path to the project file.
// It returns an error if there was an issue opening or reading the file, or if the JSON data is invalid.
func (mpc *MultipleProject) readProject(path string) error {
	jsonFilePath := filepath.Join(path, "yap.json")
	pkgbuildFilePath := filepath.Join(path, "PKGBUILD")

	var err error

	filePath, err := utils.Open(jsonFilePath)
	if err != nil {
		filePath, err = utils.Open(pkgbuildFilePath)

		mpc.setSingleProject(path)
	}

	if err != nil {
		return errors.Errorf("Unable to open any yap.json or PKGBUILD file")
	}

	defer filePath.Close()

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

// setSingleProject reads the PKGBUILD file at the given path and updates the
// MultipleProject instance.
func (mpc *MultipleProject) setSingleProject(path string) {
	cleanFilePath := filepath.Clean(path)
	proj := &Project{
		Name:           "",
		PackageManager: mpc.packageManager,
		HasToInstall:   false,
	}

	mpc.BuildDir = cleanFilePath
	mpc.Output = cleanFilePath
	mpc.Projects = append(mpc.Projects, proj)
	mpc.singleProject = true
}

// validateAllProject validates all projects in the MultipleProject struct.
//
// It takes in the distro, release, and path as parameters and returns an error.
func (mpc *MultipleProject) validateAllProject(distro, release, path string) error {
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

	return nil
}

// validateJSON validates the JSON of the MultipleProject struct.
//
// It uses the validator package to validate the struct and returns any errors encountered.
// It returns an error if the validation fails.
func (mpc *MultipleProject) validateJSON() error {
	validate := validator.New()

	return validate.Struct(mpc)
}
