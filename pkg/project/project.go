package project

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
// packages and the UntilPkgName field, which can be used to stop the build
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
	MirrorRoot     string
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
// If UntilPkgName is not empty, it stops building after the specified package.
// It returns an error if any error occurs during the build process.
func (mpc *MultipleProject) BuildAll() error {
	if UntilPkgName != "" {
		mpc.findPackageInProjects()
	}

	for _, proj := range mpc.Projects {
		fmt.Printf("%s🚀 :: %sMaking package: %s%s %s-%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			proj.Builder.PKGBUILD.PkgName,
			proj.Builder.PKGBUILD.PkgVer,
			proj.Builder.PKGBUILD.PkgRel,
		)

		if err := proj.Builder.Compile(); err != nil {
			return err
		}

		err := mpc.createPackages(proj)
		if err != nil {
			return err
		}

		if proj.HasToInstall {
			fmt.Printf("%s🤓 :: %sInstalling package: %s%s %s-%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				string(constants.ColorWhite),
				proj.Builder.PKGBUILD.PkgName,
				proj.Builder.PKGBUILD.PkgVer,
				proj.Builder.PKGBUILD.PkgRel,
			)

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

// Clean cleans up the MultipleProject by removing the package directories and
// source directories if the NoCache flag is set. It takes no parameters. It
// returns an error if there was a problem removing the directories.
func (mpc *MultipleProject) Clean() error {
	for _, project := range mpc.Projects {
		if err := utils.RemoveAll(project.Builder.PKGBUILD.PackageDir); err != nil {
			return err
		}

		if NoCache {
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

	return nil
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

// findPackageInProjects searches for a package in the MultipleProject struct.
//
// It iterates over the Projects slice and checks if the package name matches
// the value of UntilPkgName. If a match is found, it sets the matchFound variable
// to true. If no match is found, it prints an error message and exits the program.
func (mpc *MultipleProject) findPackageInProjects() {
	var matchFound bool

	for _, proj := range mpc.Projects {
		if UntilPkgName == proj.Builder.PKGBUILD.PkgName {
			matchFound = true
		}
	}

	if !matchFound {
		log.Fatalf("%s❌ :: %spackage not found: %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			UntilPkgName,
		)
	}
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

	return nil
}

// readProject reads the project file at the specified path and populates the MultipleProject struct.
//
// It takes a string parameter `path` which represents the path to the project file.
// It returns an error if there was an issue opening or reading the file, or if the JSON data is invalid.
func (mpc *MultipleProject) readProject(path string) error {
	cleanFilePath := filepath.Clean(filepath.Join(path, "yap.json"))

	filePath, err := os.Open(cleanFilePath)
	if err != nil {
		return fmt.Errorf("failed to open yap.json file within '%s': %w", cleanFilePath, err)
	}
	defer filePath.Close()

	prjContent, err := io.ReadAll(filePath)
	if err != nil {
		return fmt.Errorf("failed to read yap.json file: %w", err)
	}

	err = json.Unmarshal(prjContent, &mpc)
	if err != nil {
		return fmt.Errorf("failed to unmarshal yap.json: %w", err)
	}

	err = mpc.validateJSON()
	if err != nil {
		return fmt.Errorf("failed to validate yap.json: %w", err)
	}

	return nil
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
