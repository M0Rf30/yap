package pacman

import (
	"bytes"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/klauspost/pgzip"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/filesystem"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
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
// - artifactsPath: a string representing the path where the build artifacts will be stored.
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

	err := osutils.CreateTarZst(m.PKGBUILD.PackageDir, pkgFilePath, false)
	if err != nil {
		return err
	}

	pkgLogger := osutils.WithComponent(m.PKGBUILD.PkgName)
	pkgLogger.Info("package artifact created", osutils.Logger.Args("pkgver", m.PKGBUILD.PkgVer,
		"pkgrel", m.PKGBUILD.PkgRel,
		"artifact", pkgFilePath))

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
	m.PKGBUILD.InstalledSize, _ = osutils.GetDirSize(m.PKGBUILD.PackageDir)
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

	checksumBytes, err := osutils.CalculateSHA256(pkgBuildFile)
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

	var mtreeEntries []osutils.FileContent

	// Walk through the package directory and retrieve the contents.
	walker := filesystem.NewWalker(m.PKGBUILD.PackageDir, nil) // makepkg doesn't use backup files
	mtreeEntries, err = walker.WalkForMakePkg()
	if err != nil {
		return err // Return the error if walking the directory fails.
	}

	mtreeFile, err := renderMtree(mtreeEntries)
	if err != nil {
		log.Fatal(err)
	}

	err = createMTREEGzip(mtreeFile,
		filepath.Join(m.PKGBUILD.PackageDir, ".MTREE"))
	if err != nil {
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
// artifactsPath: the path where the package artifacts are located.
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

	err := osutils.Exec(false, "",
		"pacman",
		"-U",
		"--noconfirm",
		pkgFilePath)
	if err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Makepkg package by getting the dependencies using the PKGBUILD.
//
// makeDepends is a slice of strings representing the dependencies to be included.
// It returns an error if there is any issue getting the dependencies.
func (m *Pkg) Prepare(makeDepends []string) error {
	installArgs := getBaseInstallArgs()
	return m.PKGBUILD.GetDepends("pacman", installArgs, makeDepends)
}

// PrepareEnvironment prepares the environment for the Makepkg.
//
// It takes a boolean parameter `golang` which indicates whether the environment
// should be prepared for Golang.
// It returns an error if there is any issue in preparing the environment.
func (m *Pkg) PrepareEnvironment(golang bool) error {
	installArgs := getBaseInstallArgs()
	installArgs = append(installArgs, buildEnvironmentDeps...)

	if golang {
		osutils.CheckGO()

		installArgs = append(installArgs, "go")
	}

	return osutils.Exec(false, "", "pacman", installArgs...)
}

// Update updates the Makepkg package manager.
//
// It retrieves the updates using the GetUpdates method of the PKGBUILD struct.
// It returns an error if there is any issue during the update process.
func (m *Pkg) Update() error {
	return m.PKGBUILD.GetUpdates("pacman", "-Sy")
}

func renderMtree(entries []osutils.FileContent) (string, error) {
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
func createMTREEGzip(mtreeContent, outputFile string) error {
	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := out.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close output file", osutils.Logger.Args("error", err))
		}
	}()

	// Create a gzip writer
	gzipWriter := pgzip.NewWriter(out)

	defer func() {
		err := gzipWriter.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close gzip writer", osutils.Logger.Args("error", err))
		}
	}()

	// Copy the source file to the gzip writer
	_, err = io.Copy(gzipWriter, strings.NewReader(mtreeContent))
	if err != nil {
		return err
	}

	return nil
}
