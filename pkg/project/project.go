package project

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/builder"
	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/M0Rf30/yap/pkg/packer"
	"github.com/M0Rf30/yap/pkg/parser"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/go-playground/validator/v10"
	"github.com/otiai10/copy"
)

var (
	// CleanBuild indicates whether a clean build should be performed,
	// potentially clearing existing binaries or intermediate files.
	CleanBuild bool

	// FromPkgName is used to start the build process from a specific package.
	FromPkgName string

	// makeDepends lists packages that must be present as dependencies
	// during the build process but are not required at runtime.
	makeDepends []string

	// NoBuild specifies whether the build process should be skipped,
	// useful for debugging or when only non-compilation tasks are needed.
	NoBuild bool

	// NoMakeDeps indicates whether dependency checks and installations
	// should be bypassed during the build process.
	NoMakeDeps bool

	// packageManager is an instance of packer.Packer, used to manage
	// package operations such as installation, removal, or updates.
	packageManager packer.Packer

	// singleProject indicates whether the operation should be limited to a
	// single project or applied across multiple projects.
	singleProject bool

	// SkipSyncDeps determines whether synchronization of dependencies
	// should be skipped, potentially speeding up operations but risking
	// inconsistencies.
	SkipSyncDeps bool

	// ToPkgName is used to stop the build process after a specific package.
	ToPkgName string

	// Zap indicates whether resources should be aggressively cleaned up
	// after operations, such as removing temporary files or caches.
	Zap bool
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
	BuildDir    string     `json:"buildDir"    validate:"required"`
	Description string     `json:"description" validate:"required"`
	Name        string     `json:"name"        validate:"required"`
	Output      string     `json:"output"      validate:"required"`
	Projects    []*Project `json:"projects"    validate:"required,dive,required"`
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
	if !singleProject {
		mpc.checkPkgsRange(FromPkgName, ToPkgName)
	}

	for _, proj := range mpc.Projects {
		if FromPkgName != "" && proj.Builder.PKGBUILD.PkgName != FromPkgName {
			continue
		}

		osutils.Logger.Info("making package", osutils.Logger.Args("pkgname", proj.Builder.PKGBUILD.PkgName,
			"pkgver", proj.Builder.PKGBUILD.PkgVer,
			"pkgrel", proj.Builder.PKGBUILD.PkgRel))

		err := proj.Builder.Compile(NoBuild)
		if err != nil {
			return err
		}

		if !NoBuild {
			err := mpc.createPackage(proj)
			if err != nil {
				return err
			}
		}

		if !NoBuild && proj.HasToInstall {
			osutils.Logger.Info("installing package", osutils.Logger.Args("pkgname", proj.Builder.PKGBUILD.PkgName,
				"pkgver", proj.Builder.PKGBUILD.PkgVer,
				"pkgrel", proj.Builder.PKGBUILD.PkgRel))

			err := proj.PackageManager.Install(mpc.Output)
			if err != nil {
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
	for _, proj := range mpc.Projects {
		if CleanBuild {
			err := os.RemoveAll(proj.Builder.PKGBUILD.SourceDir)
			if err != nil {
				return err
			}
		}

		if Zap && !singleProject {
			err := os.RemoveAll(proj.Builder.PKGBUILD.StartDir)
			if err != nil {
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

	err = osutils.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	packageManager = packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
	if !SkipSyncDeps {
		err := packageManager.Update()
		if err != nil {
			return err
		}
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	if CleanBuild || Zap {
		err := mpc.Clean()
		if err != nil {
			osutils.Logger.Fatal("fatal error",
				osutils.Logger.Args("error", err))
		}
	}

	err = mpc.copyProjects()
	if err != nil {
		return err
	}

	if !NoMakeDeps {
		mpc.getMakeDeps()

		err := packageManager.Prepare(makeDepends)
		if err != nil {
			return err
		}
	}

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
		osutils.Logger.Fatal("invalid package order: %s should be built before %s",
			osutils.Logger.Args(fromPkgName, toPkgName))
	}
}

// copyProjects copies PKGBUILD directories for all projects, creating the
// target directory if it doesn't exist.
// It skips files with extensions: .apk, .deb, .pkg.tar.zst, and .rpm,
// as well as symlinks.
// Returns an error if any operation fails; otherwise, returns nil.
func (mpc *MultipleProject) copyProjects() error {
	copyOpt := copy.Options{
		OnSymlink: func(_ string) copy.SymlinkAction {
			return copy.Skip
		},
		Skip: func(_ os.FileInfo, src, _ string) (bool, error) {
			// Define a slice of file extensions to skip
			skipExtensions := []string{".apk", ".deb", ".pkg.tar.zst", ".rpm"}
			for _, ext := range skipExtensions {
				if strings.HasSuffix(src, ext) {
					return true, nil
				}
			}

			return false, nil
		},
	}

	for _, proj := range mpc.Projects {
		// Ensure the target directory exists
		err := osutils.ExistsMakeDir(proj.Builder.PKGBUILD.StartDir)
		if err != nil {
			return err
		}

		// Ensure the pkgdir directory exists
		err = osutils.ExistsMakeDir(proj.Builder.PKGBUILD.PackageDir)
		if err != nil {
			return err
		}

		// Only copy if the source and destination are different
		if !singleProject {
			err := copy.Copy(proj.Builder.PKGBUILD.Home, proj.Builder.PKGBUILD.StartDir, copyOpt)
			if err != nil {
				return err
			}
		}
	}

	return nil // Return nil if all operations succeed
}

// createPackage creates packages for the MultipleProject.
//
// It takes a pointer to a MultipleProject as a receiver and a pointer to a Project as a parameter.
// It returns an error.
func (mpc *MultipleProject) createPackage(proj *Project) error {
	if mpc.Output != "" {
		absOutput, err := filepath.Abs(mpc.Output)
		if err != nil {
			return err
		}

		mpc.Output = absOutput
	}

	defer os.RemoveAll(proj.Builder.PKGBUILD.PackageDir)

	err := osutils.ExistsMakeDir(mpc.Output)
	if err != nil {
		return err
	}

	err = proj.PackageManager.PrepareFakeroot(mpc.Output)
	if err != nil {
		return err
	}

	osutils.Logger.Info("building resulting package")

	err = proj.PackageManager.BuildPackage(mpc.Output)
	if err != nil {
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
		osutils.Logger.Fatal("package not found",
			osutils.Logger.Args("pkgname", pkgName))
	}

	return index
}

// getMakeDeps retrieves the make dependencies for the MultipleProject.
//
// It iterates over each child project and appends their make dependencies
// to the makeDepends slice. The makeDepends slice is then assigned to the
// makeDepends field of the MultipleProject.
func (mpc *MultipleProject) getMakeDeps() {
	for _, child := range mpc.Projects {
		makeDepends = append(makeDepends, child.Builder.PKGBUILD.MakeDepends...)
	}
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

		pkgbuildFile.ComputeArchitecture()
		pkgbuildFile.ValidateMandatoryItems()
		pkgbuildFile.ValidateGeneral()

		packageManager = packer.GetPackageManager(pkgbuildFile, distro)

		proj := &Project{
			Name:           child.Name,
			Builder:        &builder.Builder{PKGBUILD: pkgbuildFile},
			PackageManager: packageManager,
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

	var projectFilePath string

	if osutils.Exists(jsonFilePath) {
		projectFilePath = jsonFilePath
		osutils.Logger.Info("multi-project file found",
			osutils.Logger.Args("path", projectFilePath))
	}

	if osutils.Exists(pkgbuildFilePath) {
		projectFilePath = pkgbuildFilePath
		osutils.Logger.Info("single-project file found",
			osutils.Logger.Args("path", projectFilePath))

		mpc.setSingleProject(path)
	}

	filePath, err := osutils.Open(projectFilePath)
	if err != nil || singleProject {
		return err
	}
	defer filePath.Close()

	prjContent, err := io.ReadAll(filePath)
	if err != nil {
		return err
	}

	//nolint:musttag
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
		PackageManager: packageManager,
		HasToInstall:   false,
	}

	mpc.BuildDir = cleanFilePath
	mpc.Output = cleanFilePath
	mpc.Projects = append(mpc.Projects, proj)
	singleProject = true
}

// validateJSON validates the JSON of the MultipleProject struct.
//
// It uses the validator package to validate the struct and returns any errors encountered.
// It returns an error if the validation fails.
func (mpc *MultipleProject) validateJSON() error {
	validate := validator.New()

	return validate.Struct(mpc)
}
