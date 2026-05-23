// Package project provides multi-package project management and build orchestration.
package project

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/otiai10/copy"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/repo"
	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
)

// readProject reads the project file at the specified path
// and populates the MultipleProject struct.
//
// It takes a string parameter `path` which represents the path to the project file.
// It returns an error if there was an issue opening or reading the file, or if the
// JSON data is invalid.
func (mpc *MultipleProject) readProject(path string) error {
	jsonFilePath := filepath.Join(path, "yap.json")
	pkgbuildFilePath := filepath.Join(path, "PKGBUILD")

	var projectFilePath string

	if files.Exists(jsonFilePath) {
		projectFilePath = jsonFilePath
		logger.Debug(i18n.T("logger.multi_project_file_found"), "path", projectFilePath)
	}

	if files.Exists(pkgbuildFilePath) {
		projectFilePath = pkgbuildFilePath
		logger.Debug(i18n.T("logger.single_project_file_found"), "path", projectFilePath)

		mpc.setSingleProject(path)
	}

	filePath, err := files.Open(projectFilePath)
	if err != nil || mpc.singleProject {
		return err
	}

	defer func() {
		err := filePath.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.failed_to_close_project_file"), "path", projectFilePath, "error", err)
		}
	}()

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
		PackageManager: mpc.packageManager,
		HasToInstall:   false,
	}

	mpc.BuildDir = cleanFilePath
	mpc.Output = cleanFilePath
	mpc.Projects = append(mpc.Projects, proj)
	mpc.singleProject = true
}

// validateJSON validates the JSON of the MultipleProject struct.
//
// It uses the validator package to validate the struct and returns any errors encountered.
// It returns an error if the validation fails.
func (mpc *MultipleProject) validateJSON() error {
	return jsonValidator.Struct(mpc)
}

// populateProjects populates the MultipleProject with projects based on the
// given distro, release, and path.
//
// distro: The distribution of the projects.
// release: The release version of the projects.
// path: The path to the projects.
// error: An error if any occurred during the population process.
func (mpc *MultipleProject) populateProjects(distro, release, path string) error {
	// Resolve path to absolute so $repodir is always an absolute path in scripts.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	path = absPath

	projects := make([]*Project, 0)

	for _, child := range mpc.Projects {
		startDir := filepath.Join(mpc.BuildDir, child.Name)
		home := filepath.Join(path, child.Name)

		pkgbuildFile, err := parser.ParseFile(distro,
			release,
			startDir,
			home,
			mpc.Opts.TargetArch)
		if err != nil {
			return err
		}

		// RepoDir is the git repository root, found by walking up from the
		// yap.json directory until a .git entry is found. Empty if not in a
		// git repo. Exposed as $repodir in build/package scripts.
		pkgbuildFile.RepoDir = findGitRoot(path)

		if err := pkgbuildFile.ComputeArchitecture(); err != nil {
			return err
		}

		if err := pkgbuildFile.ValidateMandatoryItems(); err != nil {
			return err
		}

		if err := pkgbuildFile.ValidateGeneral(); err != nil {
			return err
		}

		mpc.packageManager, err = packer.GetPackageManager(pkgbuildFile, distro,
			mpc.CompressionDeb, mpc.CompressionRpm)
		if err != nil {
			return err
		}

		proj := &Project{
			Name:           child.Name,
			Builder:        &builder.Builder{PKGBUILD: pkgbuildFile, SkipHashCheck: mpc.Opts.SkipHashCheck},
			PackageManager: mpc.packageManager,
			HasToInstall:   child.HasToInstall,
		}

		projects = append(projects, proj)
	}

	mpc.Projects = projects

	// Snapshot the full project list before any filtering so getRuntimeDeps
	// can correctly exclude all yap.json packages from the apt download list,
	// regardless of which subset is selected for building.
	mpc.allProjects = append([]*Project(nil), projects...)

	// Filter projects when --only or --skip is specified
	if mpc.Opts.OnlyPkgNames != "" {
		mpc.filterProjects(mpc.Opts.OnlyPkgNames, true)
	}

	if mpc.Opts.SkipPkgNames != "" {
		mpc.filterProjects(mpc.Opts.SkipPkgNames, false)
	}

	return nil
}

// filterProjects filters mpc.Projects by a comma-separated name list.
// When keep is true (--only), only matching projects are retained.
// When keep is false (--skip), matching projects are excluded.
func (mpc *MultipleProject) filterProjects(names string, keep bool) {
	nameSet := make(map[string]struct{})

	for name := range strings.SplitSeq(names, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			nameSet[name] = struct{}{}
		}
	}

	if len(nameSet) == 0 {
		return
	}

	filtered := make([]*Project, 0, len(mpc.Projects))

	for _, proj := range mpc.Projects {
		_, matches := nameSet[proj.Builder.PKGBUILD.PkgName]
		if matches == keep {
			filtered = append(filtered, proj)
		}
	}

	logger.Info(i18n.T("logger.filtered_projects_with_only"),
		"requested", len(nameSet), "matched", len(filtered))

	mpc.Projects = filtered
}

// copyProjects copies PKGBUILD directories for all projects, creating the
// target directory if it doesn't exist.
// It skips files with extensions: .apk, .deb, .pkg.tar.zst, and .rpm,
// as well as symlinks. Uses hardlinks when possible to reduce disk usage.
// Returns an error if any operation fails; otherwise, returns nil.
func (mpc *MultipleProject) copyProjects() error {
	copyOpt := setupCopyOptions()

	for _, proj := range mpc.Projects {
		// Ensure the target directory exists
		if err := files.ExistsMakeDir(proj.Builder.PKGBUILD.StartDir); err != nil {
			return err
		}

		// Ensure the pkgdir directory exists
		if err := files.ExistsMakeDir(proj.Builder.PKGBUILD.PackageDir); err != nil {
			return err
		}

		// Only copy if StartDir and Home differ (multi-project builds use a
		// separate build dir; single-project builds use Home as StartDir directly).
		if proj.Builder.PKGBUILD.StartDir != proj.Builder.PKGBUILD.Home {
			err := copy.Copy(proj.Builder.PKGBUILD.Home, proj.Builder.PKGBUILD.StartDir, copyOpt)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// shouldSkipFile determines if a file should be skipped during copying.
func shouldSkipFile(info os.FileInfo, src, dest string) (bool, error) {
	// Skip if destination already exists with same size and modification time
	if destInfo, err := os.Stat(dest); err == nil {
		if !info.IsDir() && info.Size() == destInfo.Size() && info.ModTime().Equal(destInfo.ModTime()) {
			return true, nil
		}
	}

	// Define a slice of file extensions to skip
	skipExtensions := []string{
		".apk", ".deb", ".pkg.tar.zst", ".rpm",
		".tar.gz", ".tar.xz", ".tar.bz2", ".zip",
	}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(src, ext) {
			return true, nil
		}
	}

	// Skip temporary and build artifacts
	basename := filepath.Base(src)
	if strings.HasPrefix(basename, ".") || strings.HasSuffix(basename, ".tmp") ||
		strings.HasSuffix(basename, "~") || basename == "Thumbs.db" || basename == ".DS_Store" {
		return true, nil
	}

	return false, nil
}

// setupCopyOptions creates the copy options for the copyProjects function.
func setupCopyOptions() copy.Options {
	return copy.Options{
		OnSymlink: func(_ string) copy.SymlinkAction {
			return copy.Skip
		},
		OnDirExists: func(src, dest string) copy.DirExistsAction {
			return copy.Merge
		},
		Sync:          false, // Don't delete extra files in destination
		PreserveTimes: false, // Don't preserve modification times for better performance
		PreserveOwner: false, // Don't preserve ownership for better performance
		Skip:          shouldSkipFile,
	}
}

// findGitRoot walks up the directory tree from dir until it finds a .git
// directory (not a file — .git files are submodule markers and are skipped so
// that the top-level repository root is always returned). Falls back to the
// parent of dir when no .git directory is found — this covers CI workspaces
// where sources are copied into a staging directory without .git metadata,
// and the yap.json directory is one level below the effective repo root.
func findGitRoot(dir string) string {
	current := dir

	for {
		info, err := os.Stat(filepath.Join(current, ".git"))
		if err == nil && info.IsDir() {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding .git.
			// Fall back to the parent of the starting directory.
			return filepath.Dir(dir)
		}

		current = parent
	}
}

// applyJSONDefaults merges yap.json fields into BuildOptions.
// CLI flags take precedence: a JSON value is applied only when the
// corresponding Opts field is still at its zero value.
func (mpc *MultipleProject) applyJSONDefaults() error {
	if mpc.Opts.TargetArch == "" && mpc.TargetArch != "" {
		mpc.Opts.TargetArch = mpc.TargetArch
	}

	if mpc.Opts.DebugDir == "" && mpc.DebugDir != "" {
		mpc.Opts.DebugDir = mpc.DebugDir
	}

	if !mpc.Opts.Parallel && mpc.Parallel {
		mpc.Opts.Parallel = mpc.Parallel
	}

	if !mpc.Opts.SBOM && mpc.SBOM {
		mpc.Opts.SBOM = mpc.SBOM
	}

	if mpc.Opts.SBOMFormat == "" && mpc.SBOMFormat != "" {
		mpc.Opts.SBOMFormat = mpc.SBOMFormat
	}

	if err := common.ValidateTargetArch(mpc.Opts.TargetArch); err != nil {
		return yerrors.New(yerrors.ErrTypeConfiguration, err.Error()).
			WithOperation("applyJSONDefaults")
	}

	return nil
}

// setupExtraRepos installs custom apt/dnf repositories declared in yap.json
// (mpc.Repos) and via the repeatable --repo CLI flag (ExtraRepos). It runs
// before any package manager update so subsequent installs can resolve the new
// sources.
func (mpc *MultipleProject) setupExtraRepos(distro string) error {
	cliRepos, err := repo.ParseFlags(mpc.Opts.ExtraRepos)
	if err != nil {
		return err
	}

	merged := append([]repo.Repo{}, mpc.Repos...)
	merged = append(merged, cliRepos...)

	return repo.Setup(distro, merged)
}

// resolveOutputPath converts the output path to an absolute path.
// This must be done once before any parallel work to prevent data races.
func (mpc *MultipleProject) resolveOutputPath() error {
	if mpc.Output != "" {
		absOutput, err := filepath.Abs(mpc.Output)
		if err != nil {
			return err
		}

		mpc.Output = absOutput
	}

	return nil
}
