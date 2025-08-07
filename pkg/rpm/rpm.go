package rpm

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/rpmpack"

	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
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
		Obsoletes:   processDepends(r.PKGBUILD.Replaces),
		Provides:    processDepends(r.PKGBUILD.Provides),
		Requires:    processDepends(r.PKGBUILD.Depends),
		Conflicts:   processDepends(r.PKGBUILD.Conflicts),
		Recommends:  processDepends(r.PKGBUILD.OptDepends),
		Suggests:    processDepends(r.PKGBUILD.OptDepends),
		BuildTime:   time.Now(),
	})

	err := r.createFilesInsideRPM(rpm)
	if err != nil {
		return err
	}

	r.addScriptlets(rpm)

	cleanFilePath := filepath.Clean(pkgFilePath)

	rpmFile, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := rpmFile.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close RPM file", osutils.Logger.Args("path", cleanFilePath,
				"error", err))
		}
	}()

	err = rpm.Write(rpmFile)
	if err != nil {
		return err
	}

	pkgLogger := osutils.WithComponent(r.PKGBUILD.PkgName)
	pkgLogger.Info("package artifact created", osutils.Logger.Args("pkgver", r.PKGBUILD.PkgVer,
		"pkgrel", r.PKGBUILD.PkgRel,
		"artifact", cleanFilePath))

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

	pkgFilePath := filepath.Join(artifactsPath, pkgName)
	installArgs = append(installArgs, pkgFilePath)

	err := osutils.Exec(false, "", "dnf", installArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Prepare prepares the RPM instance by installing the required dependencies.
//
// makeDepends is a slice of strings representing the dependencies to be installed.
// It returns an error if there is any issue during the installation process.
func (r *RPM) Prepare(makeDepends []string) error {
	return r.PKGBUILD.GetDepends("dnf", installArgs, makeDepends)
}

// PrepareEnvironment prepares the environment for the RPM struct.
//
// It takes a boolean parameter `golang` which indicates whether or not to set up the
// Go environment.
// It returns an error if there was an issue with the environment preparation.
func (r *RPM) PrepareEnvironment(golang bool) error {
	installArgs = append(installArgs, buildEnvironmentDeps...)

	err := osutils.Exec(false, "", "dnf", installArgs...)
	if err != nil {
		return err
	}

	if golang {
		err = osutils.GOSetup()
		if err != nil {
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

// createFilesInsideRPM prepares and adds files to the specified RPM object.
// It retrieves backup files, walks through the package directory, and adds the contents to the RPM.
func (r *RPM) createFilesInsideRPM(rpm *rpmpack.RPM) error {
	// Prepare a list of backup files by calling the prepareBackupFiles method.
	backupFiles := r.prepareBackupFiles()

	// Walk through the package directory and retrieve the contents.
	contents, err := walkPackageDirectory(r.PKGBUILD.PackageDir, backupFiles)
	if err != nil {
		return err // Return the error if walking the directory fails.
	}

	// Add the retrieved contents to the RPM object and return any error that occurs.
	return addContentsToRPM(contents, rpm)
}

// addContentsToRPM adds a slice of FileContent objects to the specified RPM object.
// It creates RPMFile objects from the FileContent and adds them to the RPM.
func addContentsToRPM(contents []*osutils.FileContent, rpm *rpmpack.RPM) error {
	// Iterate over each FileContent in the provided slice.
	for _, content := range contents {
		// Create an RPMFile from the FileContent.
		file, err := createRPMFile(content)
		if err != nil {
			return err // Return the error if creating the RPMFile fails.
		}

		// Clean the file name to ensure it has a proper format.
		file.Name = filepath.Clean(file.Name)
		// Add the created RPMFile to the RPM object.
		rpm.AddFile(*file)
	}

	// Return nil indicating that all contents were added successfully.
	return nil
}

// addScriptlets adds pre-install, post-install, pre-remove and post-remove
// scripts from the PKGBUILD to the RPM package if they are defined.
//
// It takes a pointer to the rpmpack.RPM instance as a parameter.
func (r *RPM) addScriptlets(rpm *rpmpack.RPM) {
	// This string is appended to preun and postun directives
	// to have a similar behaviour between deb and rpm.
	onlyOnUninstall := "if [ $1 -ne 0 ]; then exit 0; fi\n"

	if r.PKGBUILD.PreTrans != "" {
		rpm.AddPretrans(r.PKGBUILD.PreTrans)
	}

	if r.PKGBUILD.PreInst != "" {
		rpm.AddPrein(r.PKGBUILD.PreInst)
	}

	if r.PKGBUILD.PostInst != "" {
		rpm.AddPostin(r.PKGBUILD.PostInst)
	}

	if r.PKGBUILD.PreRm != "" {
		rpm.AddPreun(onlyOnUninstall + r.PKGBUILD.PreRm)
	}

	if r.PKGBUILD.PostRm != "" {
		rpm.AddPostun(onlyOnUninstall + r.PKGBUILD.PostRm)
	}

	if r.PKGBUILD.PostTrans != "" {
		rpm.AddPosttrans(r.PKGBUILD.PostTrans)
	}
}

// asRPMDirectory creates an RPMFile object for a directory based on the provided FileContent.
// It retrieves the directory's modification time and sets the appropriate fields in the RPMFile.
func asRPMDirectory(content *osutils.FileContent) *rpmpack.RPMFile {
	// Get file information for the directory specified in the content.
	fileInfo, _ := os.Stat(filepath.Clean(content.Source))

	// Retrieve the modification time of the directory.
	mTime := getModTime(fileInfo)

	// Create and return an RPMFile object for the directory.
	return &rpmpack.RPMFile{
		Name:  content.Destination,                          // Set the destination name.
		Mode:  uint(fileInfo.Mode()) | osutils.TagDirectory, // Set the mode to indicate it's a directory.
		MTime: mTime,                                        // Set the modification time.
		Owner: "root",                                       // Set the owner to "root".
		Group: "root",                                       // Set the group to "root".
	}
}

// asRPMFile creates an RPMFile object for a regular file based on the provided FileContent.
// It reads the file's data and retrieves its modification time.
func asRPMFile(content *osutils.FileContent, fileType rpmpack.FileType) (*rpmpack.RPMFile, error) {
	// Read the file data from the source path.
	data, err := os.ReadFile(content.Source)
	if err != nil {
		return nil, err // Return nil and the error if reading the file fails.
	}

	cleanFilePath := filepath.Clean(content.Source)
	fileInfo, _ := os.Stat(cleanFilePath)

	// Retrieve the modification time of the file.
	mTime := getModTime(fileInfo)

	// Create and return an RPMFile object for the regular file.
	return &rpmpack.RPMFile{
		Name:  content.Destination,   // Set the destination name.
		Body:  data,                  // Set the file data.
		Mode:  uint(fileInfo.Mode()), // Set the file mode.
		MTime: mTime,                 // Set the modification time.
		Owner: "root",                // Set the owner to "root".
		Group: "root",                // Set the group to "root".
		Type:  fileType,              // Set the file type.
	}, nil
}

// asRPMSymlink creates an RPMFile object for a symbolic link based on the provided FileContent.
// It retrieves the link's target and modification time.
func asRPMSymlink(content *osutils.FileContent) *rpmpack.RPMFile {
	cleanFilePath := filepath.Clean(content.Source)
	fileInfo, _ := os.Lstat(cleanFilePath) // Use Lstat to get information about the symlink.
	body, _ := os.Readlink(cleanFilePath)  // Read the target of the symlink.

	// Retrieve the modification time of the symlink.
	mTime := getModTime(fileInfo)

	// Create and return an RPMFile object for the symlink.
	return &rpmpack.RPMFile{
		Name:  content.Destination,   // Set the destination name.
		Body:  []byte(body),          // Set the target of the symlink as the body.
		Mode:  uint(osutils.TagLink), // Set the mode to indicate it's a symlink.
		MTime: mTime,                 // Set the modification time.
		Owner: "root",                // Set the owner to "root".
		Group: "root",                // Set the group to "root".
	}
}

// createContent creates a new FileContent object with the specified source path,
// destination path (relative to the package directory), and content type.
func createContent(path, packageDir, contentType string) *osutils.FileContent {
	return &osutils.FileContent{
		Source:      path,
		Destination: strings.TrimPrefix(path, packageDir),
		Type:        contentType,
	}
}

// createRPMFile converts a FileContent object into an RPMFile object based on its type.
// It returns the created RPMFile and any error encountered during the conversion.
func createRPMFile(content *osutils.FileContent) (*rpmpack.RPMFile, error) {
	var file *rpmpack.RPMFile

	var err error

	switch content.Type {
	case osutils.TypeConfigNoReplace:
		file, err = asRPMFile(content, rpmpack.ConfigFile|rpmpack.NoReplaceFile)
	case osutils.TypeSymlink:
		file = asRPMSymlink(content)
	case osutils.TypeDir:
		file = asRPMDirectory(content)
	case osutils.TypeFile:
		file, err = asRPMFile(content, rpmpack.GenericFile)
	}

	return file, err
}

// getGroup updates the section of the RPM struct with the corresponding
// value from the RPMGroups map.
//
// No parameters.
// No return types.
func (r *RPM) getGroup() {
	r.PKGBUILD.Section = RPMGroups[r.PKGBUILD.Section]
}

// getModTime retrieves the modification time of a file and checks for overflow.
// It returns the modification time as an uint32.
func getModTime(fileInfo os.FileInfo) uint32 {
	mTime := fileInfo.ModTime().Unix()
	// Check for overflow in the modification time.
	if mTime < 0 || mTime > int64(^uint32(0)) {
		osutils.Logger.Fatal("modification time is out of range for uint32",
			osutils.Logger.Args("time", mTime))
	}

	return uint32(mTime)
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

// handleFileEntry processes a file entry at the given path, checking if it is a backup file,
// and appending its content to the provided slice based on its type (config, symlink, or
// egular file).
func handleFileEntry(path string, backupFiles []string,
	packageDir string, contents *[]*osutils.FileContent,
) error {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return err // Handle error from os.Lstat
	}

	switch {
	case fileInfo.Mode()&os.ModeSymlink != 0:
		*contents = append(*contents, createContent(path, packageDir, osutils.TypeSymlink))
	case osutils.Contains(backupFiles, strings.TrimPrefix(path, packageDir)):
		*contents = append(*contents, createContent(path, packageDir, osutils.TypeConfigNoReplace))
	default:
		*contents = append(*contents, createContent(path, packageDir, osutils.TypeFile))
	}

	return nil
}

// prepareBackupFiles prepares a list of backup file paths by ensuring each path
// has a leading slash and returns the resulting slice of backup file paths.
func (r *RPM) prepareBackupFiles() []string {
	backupFiles := make([]string, 0)

	for _, filePath := range r.PKGBUILD.Backup {
		if !strings.HasPrefix(filePath, "/") {
			filePath = "/" + filePath
		}

		backupFiles = append(backupFiles, filePath)
	}

	return backupFiles
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

		err := relations.Set(depends[index])
		if err != nil {
			return nil
		}
	}

	return relations
}

// walkPackageDirectory traverses the specified package directory and collects
// file contents, including handling backup files and empty directories.
// It returns a slice of FileContent and an error if any occurs during the traversal.
func walkPackageDirectory(packageDir string, backupFiles []string) ([]*osutils.FileContent, error) {
	var contents []*osutils.FileContent

	err := filepath.WalkDir(packageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if dirEntry.IsDir() {
			if osutils.IsEmptyDir(path, dirEntry) {
				contents = append(contents, createContent(path, packageDir, osutils.TypeDir))
			}

			return nil
		}

		return handleFileEntry(path, backupFiles, packageDir, &contents)
	})
	if err != nil {
		return nil, err
	}

	return contents, nil
}
