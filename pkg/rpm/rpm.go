package rpm

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/M0Rf30/yap/pkg/options"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/google/rpmpack"
)

// RPM represents a RPM package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type RPM struct {
	PKGBUILD *pkgbuild.PKGBUILD
}

// BuildPackage creates an RPM package based on the provided PKGBUILD information.
func (r *RPM) BuildPackage(artifactsPath string) error {
	pkgName := r.PKGBUILD.PkgName +
		"-" +
		r.PKGBUILD.PkgVer +
		"-" +
		r.PKGBUILD.PkgRel +
		"." +
		r.PKGBUILD.ArchComputed +
		".rpm"

	epoch, _ := strconv.ParseUint(r.PKGBUILD.Epoch, 10, 32)
	if epoch == 0 {
		epoch = uint64(rpmpack.NoEpoch)
	}

	copyright := strings.Join(r.PKGBUILD.Copyright, "; ")
	copyright = strings.TrimSuffix(copyright, " ")
	license := strings.Join(r.PKGBUILD.License, " ")

	pkgFilePath := filepath.Join(artifactsPath, pkgName)
	rpm, _ := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:        r.PKGBUILD.PkgName,
		Summary:     r.PKGBUILD.PkgDesc,
		Description: r.PKGBUILD.PkgDesc,
		Epoch:       uint32(epoch),
		Version:     r.PKGBUILD.PkgVer,
		Release:     r.PKGBUILD.PkgRel,
		Arch:        r.PKGBUILD.ArchComputed,
		Vendor:      copyright,
		URL:         r.PKGBUILD.URL,
		Packager:    r.PKGBUILD.Maintainer,
		Group:       r.PKGBUILD.Section,
		Compressor:  "zstd",
		Licence:     license,
		Provides:    processDepends(r.PKGBUILD.Provides),
		Requires:    processDepends(r.PKGBUILD.Depends),
		Conflicts:   processDepends(r.PKGBUILD.Conflicts),
		Recommends:  processDepends(r.PKGBUILD.OptDepends),
		Suggests:    processDepends(r.PKGBUILD.OptDepends),
		BuildTime:   time.Now(),
	})

	r.addScriptFiles(rpm)

	if err := r.addFiles(rpm); err != nil {
		return err
	}

	cleanFilePath := filepath.Clean(pkgFilePath)

	rpmFile, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}
	defer rpmFile.Close()

	if err := rpm.Write(rpmFile); err != nil {
		return err
	}

	return nil
}

// PrepareFakeroot sets up the environment for building an RPM package in a fakeroot context.
// It retrieves architecture, group, and release information, sets the package destination,
// cleans up the RPM directory, creates necessary directories, and gathers files.
// It also processes package dependencies and creates the RPM spec file, returning
// an error if any step fails.
func (r *RPM) PrepareFakeroot(_ string) error {
	r.getGroup()
	r.getRelease()
	r.PKGBUILD.ArchComputed = RPMArchs[r.PKGBUILD.ArchComputed]

	if r.PKGBUILD.StripEnabled {
		return options.Strip(r.PKGBUILD.PackageDir)
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
	pkgName := r.PKGBUILD.PkgName +
		"-" +
		r.PKGBUILD.PkgVer +
		"-" +
		r.PKGBUILD.PkgRel +
		"." +
		r.PKGBUILD.ArchComputed +
		".rpm"

	pkgFilePath := filepath.Join(artifactsPath, r.PKGBUILD.ArchComputed, pkgName)

	if err := utils.Exec(false, "",
		"dnf",
		"install",
		"-y",
		pkgFilePath); err != nil {
		return err
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

	if err := utils.Exec(false, "", "dnf", args...); err != nil {
		return err
	}

	if golang {
		if err := utils.GOSetup(); err != nil {
			return err
		}
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

// addFiles adds files from the specified package directory to the RPM package.
func (r *RPM) addFiles(rpm *rpmpack.RPM) error {
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
		cleanFilePath := filepath.Clean(filePath)
		body, _ := os.ReadFile(cleanFilePath)
		rpm.AddFile(rpmpack.RPMFile{
			Name: strings.TrimPrefix(filePath, r.PKGBUILD.PackageDir),
			Body: body,
		})
	}

	return nil
}

// addScriptFiles adds script files to the RPM package based on the PKGBUILD configuration.
func (r *RPM) addScriptFiles(rpm *rpmpack.RPM) {
	if r.PKGBUILD.PreInst != "" {
		rpm.AddPretrans(r.PKGBUILD.PreInst)
	}

	if r.PKGBUILD.PreRm != "" {
		rpm.AddPretrans(r.PKGBUILD.PreRm)
	}

	if r.PKGBUILD.PostInst != "" {
		rpm.AddPretrans(r.PKGBUILD.PostInst)
	}

	if r.PKGBUILD.PostRm != "" {
		rpm.AddPretrans(r.PKGBUILD.PostRm)
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

// processDepends converts a slice of strings into a rpmpack.Relations object.
// It attempts to set each string in the slice as a relation.
// If any error occurs during the setting process, it returns nil.
func processDepends(depends []string) rpmpack.Relations {
	pattern := `(?m)(<|<=|>=|=|>|<)`
	regex := regexp.MustCompile(pattern)
	relations := make(rpmpack.Relations, 0)

	for index, depend := range depends {
		result := regex.Split(depend, -1)
		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]
			depends[index] = name + " " + operator + " " + version
		}

		if err := relations.Set(depends[index]); err != nil {
			return nil
		}
	}

	return relations
}
