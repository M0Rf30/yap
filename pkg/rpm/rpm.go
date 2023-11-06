package rpm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/set"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/otiai10/copy"
)

// RPM represents a RPM package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type RPM struct {
	PKGBUILD     *pkgbuild.PKGBUILD
	RPMDir       string
	buildDir     string
	buildRootDir string
	rpmsDir      string
	sourcesDir   string
	specsDir     string
	srpmsDir     string
}

// Build builds the RPM package.
//
// It takes the artifactsPath as a parameter and returns an error.
func (r *RPM) Build(artifactsPath string) error {
	r.getArch()
	r.getGroup()
	r.getRelease()

	r.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)

	err := utils.RemoveAll(r.RPMDir)
	if err != nil {
		return err
	}

	err = r.makeDirs()
	if err != nil {
		return err
	}

	err = r.getFiles()
	if err != nil {
		return err
	}

	buildRootPackageDir := fmt.Sprintf("%s/%s-%s-%s.%s",
		r.buildRootDir,
		r.PKGBUILD.PkgName,
		r.PKGBUILD.PkgVer,
		r.PKGBUILD.PkgRel,
		r.PKGBUILD.Arch[0])

	err = copy.Copy(r.PKGBUILD.PackageDir, buildRootPackageDir)
	if err != nil {
		return err
	}

	err = r.PKGBUILD.CreateSpec(filepath.Join(r.specsDir,
		r.PKGBUILD.PkgName+".spec"), specFile)
	if err != nil {
		return err
	}

	err = r.rpmBuild()
	if err != nil {
		return err
	}

	return nil
}

// Install installs the RPM package to the specified artifacts path.
//
// It takes the following parameter:
// - artifactsPath: The path to the directory where the artifacts are stored.
//
// It returns an error if there was an issue during the installation process.
func (r *RPM) Install(artifactsPath string) error {
	for _, arch := range r.PKGBUILD.Arch {
		pkgName := r.PKGBUILD.PkgName +
			"-" +
			r.PKGBUILD.PkgVer +
			"-" +
			r.PKGBUILD.PkgRel +
			"." +
			RPMArchs[arch] +
			".rpm"

		pkgFilePath := filepath.Join(artifactsPath, RPMArchs[arch], pkgName)

		if err := utils.Exec("",
			"yum",
			"install",
			"-y",
			pkgFilePath); err != nil {
			return err
		}
	}

	return nil
}

// PrepareEnvironment prepares the environment for the RPM struct.
//
// It takes a boolean parameter `golang` which indicates whether or not to set up the Go environment.
// It returns an error if there was an issue with the environment preparation.
func (r *RPM) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"-y",
		"install",
	}
	args = append(args, buildEnvironmentDeps...)

	err = utils.Exec("", "yum", args...)

	if err != nil {
		return err
	}

	if golang {
		utils.GOSetup()
	}

	return nil
}

// Prepare prepares the RPM instance by installing the required dependencies.
//
// makeDepends is a slice of strings representing the dependencies to be installed.
// It returns an error if there is any issue during the installation process.
func (r *RPM) Prepare(makeDepends []string) error {
	args := []string{
		"-y",
		"install",
	}

	err := r.PKGBUILD.GetDepends("dnf", args, makeDepends)
	if err != nil {
		return err
	}

	return nil
}

// Update updates the RPM object.
//
// It takes no parameters.
// It returns an error.
func (r *RPM) Update() error {
	return nil
}

// getFiles retrieves the files from the RPM package directory and populates the PKGBUILD.Files field.
//
// It iterates over the files in the package directory and adds them to the PKGBUILD.Files slice.
// It also handles the backup paths specified in the PKGBUILD.Backup field.
//
// Returns an error if there is any issue while walking the directory or retrieving the files.
func (r *RPM) getFiles() error {
	backup := set.NewSet()
	paths := set.NewSet()

	for _, path := range r.PKGBUILD.Backup {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		backup.Add(path)
	}

	var files []string

	err := filepath.Walk(r.PKGBUILD.PackageDir,
		func(path string,
			info os.FileInfo, err error) error {
			if !info.IsDir() {
				files = append(files, path)
			}

			return err
		})

	if err != nil {
		return err
	}

	for _, filePath := range files {
		if len(filePath) < 1 ||
			strings.Contains(filePath, ".build-id") {
			continue
		}

		paths.Remove(filepath.Dir(filePath))
		paths.Add(strings.TrimPrefix(filePath, r.PKGBUILD.PackageDir))
	}

	for pathInf := range paths.Iter() {
		if !strings.HasPrefix(pathInf, "/") {
			pathInf = "/" + pathInf
		}

		if backup.Contains(pathInf) {
			pathInf = `%config "` + pathInf + `"`
		} else {
			pathInf = `"` + pathInf + `"`
		}

		r.PKGBUILD.Files = append(r.PKGBUILD.Files, pathInf)
	}

	return nil
}

// getArch updates the architecture values in the RPM struct.
//
// It does not take any parameters.
// It does not return anything.
func (r *RPM) getArch() {
	for index, arch := range r.PKGBUILD.Arch {
		r.PKGBUILD.Arch[index] = RPMArchs[arch]
	}
}

// getGroup updates the section of the RPM struct with the corresponding
// value from the RPMGroups map.
//
// No parameters.
// No return types.
func (r *RPM) getGroup() {
	r.PKGBUILD.Section = RPMGroups[r.PKGBUILD.Section]
}

// getRelease updates the release information of the RPM struct.
//
// It appends the RPMDistros[r.PKGBUILD.Distro] and r.PKGBUILD.Codename to
// r.PKGBUILD.PkgRel if r.PKGBUILD.Codename is not empty.
func (r *RPM) getRelease() {
	if r.PKGBUILD.Codename != "" {
		r.PKGBUILD.PkgRel = r.PKGBUILD.PkgRel +
			RPMDistros[r.PKGBUILD.Distro] +
			r.PKGBUILD.Codename
	}
}

// makeDirs creates the necessary directories for the RPM struct.
//
// It does not take any parameters.
// It returns an error if any directory creation fails.
func (r *RPM) makeDirs() error {
	var err error

	r.RPMDir = filepath.Join(r.PKGBUILD.StartDir, "RPM")
	r.buildDir = filepath.Join(r.RPMDir, "BUILD")
	r.buildRootDir = filepath.Join(r.RPMDir, "BUILDROOT")
	r.rpmsDir = filepath.Join(r.RPMDir, "RPMS")
	r.sourcesDir = filepath.Join(r.RPMDir, "SOURCES")
	r.specsDir = filepath.Join(r.RPMDir, "SPECS")
	r.srpmsDir = filepath.Join(r.RPMDir, "SRPMS")

	for _, path := range []string{
		r.RPMDir,
		r.buildDir,
		r.buildRootDir,
		r.rpmsDir,
		r.sourcesDir,
		r.specsDir,
		r.srpmsDir,
	} {
		err = utils.ExistsMakeDir(path)
		if err != nil {
			return err
		}
	}

	return nil
}

// rpmBuild builds an RPM package using the RPM package manager.
//
// It executes the 'rpmbuild' command with the necessary options and arguments
// to build the RPM package. The package is built using the specified
// specifications file and the resulting package is stored in the RPM directory.
//
// Returns an error if the 'rpmbuild' command fails to execute or if there
// are any errors during the package building process.
func (r *RPM) rpmBuild() error {
	err := utils.Exec(r.specsDir,
		"rpmbuild",
		"--define",
		"_topdir "+
			r.RPMDir,
		"-bb",
		r.PKGBUILD.PkgName+
			".spec")
	if err != nil {
		return err
	}

	return nil
}
