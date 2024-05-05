package rpm

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
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

	if err := utils.RemoveAll(r.RPMDir); err != nil {
		return err
	}

	if err := r.makeDirs(); err != nil {
		return err
	}

	if err := r.getFiles(); err != nil {
		return err
	}

	buildRootPackageDir := fmt.Sprintf("%s/%s-%s-%s.%s",
		r.buildRootDir,
		r.PKGBUILD.PkgName,
		r.PKGBUILD.PkgVer,
		r.PKGBUILD.PkgRel,
		r.PKGBUILD.Arch[0])

	if err := copy.Copy(r.PKGBUILD.PackageDir, buildRootPackageDir); err != nil {
		return err
	}

	r.PKGBUILD.Depends = r.processDepends(r.PKGBUILD.Depends)
	r.PKGBUILD.MakeDepends = r.processDepends(r.PKGBUILD.MakeDepends)
	r.PKGBUILD.OptDepends = r.processDepends(r.PKGBUILD.OptDepends)

	tmpl := r.PKGBUILD.RenderSpec(specFile)
	if err := r.PKGBUILD.CreateSpec(filepath.Join(r.specsDir,
		r.PKGBUILD.PkgName+".spec"), tmpl); err != nil {
		return err
	}

	if err := r.rpmBuild(); err != nil {
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

		if err := utils.Exec(false, "",
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
	args := []string{
		"-y",
		"install",
	}
	args = append(args, buildEnvironmentDeps...)

	if err := utils.Exec(false, "", "yum", args...); err != nil {
		return err
	}

	if golang {
		if err := utils.GOSetup(); err != nil {
			return err
		}
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

	return r.PKGBUILD.GetDepends("dnf", args, makeDepends)
}

// Update updates the RPM object.
//
// It takes no parameters.
// It returns an error.
func (r *RPM) Update() error {
	return nil
}

// getFiles retrieves a list of files from the RPM struct.
//
// This function iterates over the list of backup paths in the PKGBUILD struct
// and adds them to the "backup" set. It then uses filepath.WalkDir to walk
// through the files in the PKGBUILD.PackageDir directory. For each file, it
// checks if it is a directory or a regular file. If it is a directory, it
// checks if it is an empty directory and adds it to the "files" list if it is.
// If it is a regular file, it adds it to the "files" list. After collecting all
// the file paths, it removes the parent directories of each file path from the
// "paths" set and adds the remaining file paths to the "PKGBUILD.Files" list in
// the RPM struct. If a file path is found in the "backup" set, it is marked as
// a config file and its path is prefixed with "%config". Finally, it returns
// nil, indicating that no error occurred.
//
// No parameters.
// Returns an error if there was an issue retrieving the files.
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

	err := filepath.WalkDir(r.PKGBUILD.PackageDir,
		func(path string, dirEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if dirEntry.IsDir() {
				isEmptyDir := utils.IsEmptyDir(path, dirEntry)
				if isEmptyDir {
					files = append(files, path)
				}
			} else {
				files = append(files, path)
			}

			return nil
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
		if pathInf != "" {
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
		if err := utils.ExistsMakeDir(path); err != nil {
			return err
		}
	}

	return nil
}

// processDepends takes a slice of strings and processes each string in order to
// modify it and return a new slice of strings for rpm syntax.
//
// It splits each string into three parts: name, operator, and version. If the
// string is split successfully, it combines the three parts into a new format
// and replaces the original string in the slice.
//
// Parameters:
//   - depends: a slice of strings to be processed.
//
// Returns:
//   - a new slice of strings with modified elements for rpm syntax.
func (r *RPM) processDepends(depends []string) []string {
	pattern := `(?m)(<|<=|>=|=|>|<)`
	regex := regexp.MustCompile(pattern)

	for index, depend := range depends {
		result := regex.Split(depend, -1)

		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]
			depends[index] = name + " " + operator + " " + version
		}
	}

	return depends
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
	args := []string{
		"--define",
		"_topdir " +
			r.RPMDir,
		"-bb",
		r.PKGBUILD.PkgName +
			".spec",
	}

	if pkgbuild.Verbose {
		args = append(args, "--verbose")
	} else {
		args = append(args, "--quiet")
	}

	return utils.Exec(false, r.specsDir, "rpmbuild", args...)
}
