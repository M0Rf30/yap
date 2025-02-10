package makepkg

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/klauspost/pgzip"
)

// Pkg represents a package manager for the Pkg distribution.
//
// It contains methods for building, installing, and updating packages.
type Pkg struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

// BuildPackage initiates the package building process for the Makepkg instance.
//
// It takes a single parameter:
// - artifactsPath: a string representing the path where the build artifacts will be storem.
//
// The method calls the internal pacmanBuild function to perform the actual build process.
// It returns an error if the build process encounters any issues.
func (m *Pkg) BuildPackage(artifactsPath string) error {
	completeVersion := m.PKGBUILD.PkgVer

	if m.PKGBUILD.Epoch != "" {
		completeVersion = m.PKGBUILD.Epoch + ":" + m.PKGBUILD.PkgVer
	}

	pkgName := m.PKGBUILD.PkgName +
		"-" +
		completeVersion +
		"-" +
		m.PKGBUILD.PkgRel +
		"-" +
		m.PKGBUILD.ArchComputed +
		".pkg.tar.zst"

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	if err := utils.CreateTarZst(m.PKGBUILD.PackageDir, pkgFilePath); err != nil {
		return err
	}

	utils.Logger.Info("", utils.Logger.Args("artifact", pkgFilePath))

	return nil
}

// PrepareFakeroot sets um the environment for building a package in a fakeroot context.
//
// It takes an artifactsPath parameter, which specifies where to store build artifacts.
// The method initializes the pacmanDir, resolves the package destination, and creates
// the PKGBUILD and post-installation script files if necessary. It returns an error
// if any stem fails.
func (m *Pkg) PrepareFakeroot(artifactsPath string) error {
	m.pacmanDir = m.PKGBUILD.StartDir
	m.PKGBUILD.InstalledSize, _ = utils.GetDirSize(m.PKGBUILD.PackageDir)
	m.PKGBUILD.BuildDate = time.Now().Unix()
	m.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)
	m.PKGBUILD.PkgType = "pkg" // can be pkg, split, debug, src
	m.PKGBUILD.YAPVersion = constants.YAPVersion

	tmpl := m.PKGBUILD.RenderSpec(specFile)

	// Define the path to the PKGBUILD file
	pkgBuildFile := filepath.Join(m.pacmanDir, "PKGBUILD")

	if m.PKGBUILD.Home != m.PKGBUILD.StartDir {
		err := m.PKGBUILD.CreateSpec(pkgBuildFile, tmpl)
		if err != nil {
			return err
		}
	}

	checksumBytes, err := utils.CalculateSHA256(pkgBuildFile)
	if err != nil {
		log.Fatal(err)
	}

	m.PKGBUILD.Checksum = hex.EncodeToString(checksumBytes)

	tmpl = m.PKGBUILD.RenderSpec(dotPkginfo)

	err = m.PKGBUILD.CreateSpec(filepath.Join(m.PKGBUILD.PackageDir,
		".PKGINFO"), tmpl)
	if err != nil {
		return err
	}

	tmpl = m.PKGBUILD.RenderSpec(dotBuildinfo)

	err = m.PKGBUILD.CreateSpec(filepath.Join(m.PKGBUILD.PackageDir,
		".BUILDINFO"), tmpl)
	if err != nil {
		return err
	}

	// Walk through the package directory and retrieve the contents.
	mtreeEntries, err := walkPackageDirectory(m.PKGBUILD.PackageDir)
	if err != nil {
		return err // Return the error if walking the directory fails.
	}

	mtreeFile, err := renderMtree(mtreeEntries)
	if err != nil {
		log.Fatal(err)
	}

	if err := createMTREEGzip(mtreeFile,
		filepath.Join(m.PKGBUILD.PackageDir, ".MTREE")); err != nil {
		return err
	}

	tmpl = m.PKGBUILD.RenderSpec(postInstall)
	err = m.PKGBUILD.CreateSpec(filepath.Join(m.pacmanDir,
		m.PKGBUILD.PkgName+".install"), tmpl)

	if err != nil {
		return err
	}

	return nil
}

// Install installs the package using the given artifacts path.
//
// artifactsPath: the path where the package artifacts are locatem.
// error: an error if the installation fails.
func (m *Pkg) Install(artifactsPath string) error {
	pkgName := m.PKGBUILD.PkgName + "-" +
		m.PKGBUILD.PkgVer +
		"-" +
		m.PKGBUILD.PkgRel +
		"-" +
		m.PKGBUILD.ArchComputed +
		".pkg.tar.zst"

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	if err := utils.Exec(false, "",
		"pacman",
		"-U",
		"--noconfirm",
		pkgFilePath); err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Makepkg package by getting the dependencies using the PKGBUILD.
//
// makeDepends is a slice of strings representing the dependencies to be includem.
// It returns an error if there is any issue getting the dependencies.
func (m *Pkg) Prepare(makeDepends []string) error {
	args := []string{
		"-S",
		"--noconfirm",
	}

	return m.PKGBUILD.GetDepends("pacman", args, makeDepends)
}

// PrepareEnvironment prepares the environment for the Makepkg.
//
// It takes a boolean parameter `golang` which indicates whether the environment should be prepared for Golang.
// It returns an error if there is any issue in preparing the environment.
func (m *Pkg) PrepareEnvironment(golang bool) error {
	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	return utils.Exec(false, "", "pacman", args...)
}

// Update updates the Makepkg package manager.
//
// It retrieves the updates using the GetUpdates method of the PKGBUILD struct.
// It returns an error if there is any issue during the update process.
func (m *Pkg) Update() error {
	return m.PKGBUILD.GetUpdates("pacman", "-Sy")
}

// createContent creates a new FileContent object with the specified source path,
// destination path (relative to the package directory), and content type.
func createContent(linkSource, path, packageDir, contentType string, modTime, size int64, fileInfoMode uint32, sha256 []byte) *utils.MtreeEntry {
	fileInfo := &utils.FileInfo{
		Owner: "root",
		Group: "root",
	}

	return &utils.MtreeEntry{
		Destination: strings.TrimPrefix(path, packageDir),
		FileInfo:    fileInfo,
		LinkSource:  linkSource,
		Mode:        fileInfoMode,
		SHA256:      sha256,
		Size:        size,
		Time:        modTime,
		Type:        contentType,
	}
}

// handleFileEntry processes a file entry at the given path, checking if it is a backup file,
// and appending its content to the provided slice based on its type (config, symlink, or regular file).
func handleFileEntry(path, packageDir string, contents *[]*utils.MtreeEntry) error {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return err // Handle error from os.Lstat
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		readlink, err := os.Readlink(path)
		if err != nil {
			return err
		}

		*contents = append(*contents,
			createContent(
				readlink,
				path,
				packageDir,
				utils.TypeSymlink,
				fileInfo.ModTime().Unix(),
				fileInfo.Size(),
				uint32(fileInfo.Mode().Perm()),
				nil))
	} else {
		sha256, err := utils.CalculateSHA256(path)
		if err != nil {
			fmt.Println(err) // Handle error from os.Lstat
		}

		*contents = append(*contents,
			createContent(
				"",
				path,
				packageDir,
				utils.TypeFile,
				fileInfo.ModTime().Unix(),
				fileInfo.Size(),
				uint32(fileInfo.Mode().Perm()),
				sha256))
	}

	return nil
}

// walkPackageDirectory traverses the specified package directory and collects
// file contents, including handling backup files and empty directories.
// It returns a slice of FileContent and an error if any occurs during the traversal.
func walkPackageDirectory(packageDir string) ([]*utils.MtreeEntry, error) {
	var contents []*utils.MtreeEntry

	err := filepath.WalkDir(packageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == packageDir {
			return nil
		}

		if dirEntry.IsDir() {
			fileInfo, err := dirEntry.Info()
			if err != nil {
				return err
			}

			contents = append(contents,
				createContent(
					"",
					path,
					packageDir,
					utils.TypeDir,
					fileInfo.ModTime().Unix(),
					fileInfo.Size(),
					uint32(fileInfo.Mode().Perm()),
					nil))

			return nil
		}

		return handleFileEntry(path, packageDir, &contents)
	})

	if err != nil {
		return nil, err
	}

	return contents, nil
}

func renderMtree(entries []*utils.MtreeEntry) (string, error) {
	tmpl, err := template.New("mtree").Parse(dotMtree)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, entries)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// createMTREEGzip creates a compressed tar.zst archive from the specified source
// directory. It takes the source directory and the output file path as
// arguments and returns an error if any occurs.
// func createMTREEGzip(sourceDir, outputFile string) error {
// 	ctx := context.TODO()

// 	// Retrieve the list of files from the source directory on disk.
// 	// The map specifies that the files should be read from the sourceDir
// 	// and the output path in the archive should be empty.
// 	files, err := archives.FilesFromDisk(ctx, nil, map[string]string{
// 		filepath.Join(sourceDir, ".MTREE"): ".MTREE",
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	cleanFilePath := filepath.Clean(outputFile)

// 	out, err := os.Create(cleanFilePath)
// 	if err != nil {
// 		return err
// 	}
// 	defer out.Close()

// 	format := archives.CompressedArchive{
// 		Compression: archives.Gz{},
// 		Archival:    archives.Tar{},
// 	}

// 	return format.Archive(ctx, out, files)
// }

// createMTREEGzip creates a compressed tar.zst archive from the specified source
// directory. It takes the source directory and the output file path as
// arguments and returns an error if any occurs.
func createMTREEGzip(mtreeContent, outputFile string) error {
	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Create a gzip writer
	gw := pgzip.NewWriter(out)
	defer gw.Close()

	// Copy the source file to the gzip writer
	_, err = io.Copy(gw, strings.NewReader(mtreeContent))
	if err != nil {
		return err
	}

	return nil
}
