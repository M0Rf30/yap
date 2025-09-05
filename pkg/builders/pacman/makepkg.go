package pacman

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/klauspost/pgzip"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/crypto"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Pkg represents a package manager for the Pkg distribution.
//
// It contains methods for building, installing, and updating packages.
type Pkg struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

// NewBuilder creates a new Pacman package builder.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *Pkg {
	return &Pkg{
		PKGBUILD: pkgBuild,
	}
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
		completeVersion = fmt.Sprintf("%s:%s", m.PKGBUILD.Epoch, m.PKGBUILD.PkgVer)
	}

	pkgName := fmt.Sprintf("%s-%s-%s-%s.pkg.tar.zst",
		m.PKGBUILD.PkgName,
		completeVersion,
		m.PKGBUILD.PkgRel,
		m.PKGBUILD.ArchComputed)

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := archive.CreateTarZst(m.PKGBUILD.PackageDir, pkgFilePath, false)
	if err != nil {
		return err
	}

	logger.Info(i18n.T("logger.buildpackage.info.package_artifact_created_1"),
		"package", m.PKGBUILD.PkgName,
		"version", m.PKGBUILD.PkgVer,
		"release", m.PKGBUILD.PkgRel,
		"artifact", pkgFilePath)

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
	m.PKGBUILD.InstalledSize, _ = files.GetDirSize(m.PKGBUILD.PackageDir)
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

	checksumBytes, err := crypto.CalculateSHA256(pkgBuildFile)
	if err != nil {
		return err
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

	var mtreeEntries []*files.Entry

	// Create file walker
	walker := files.NewWalker(m.PKGBUILD.PackageDir, files.WalkOptions{
		SkipDotFiles: true, // makepkg skips dot files
	})

	// Walk through the package directory and retrieve the contents.
	entries, err := walker.Walk()
	if err != nil {
		return err // Return the error if walking the directory fails.
	}

	// Use entries directly
	mtreeEntries = entries

	mtreeFile, err := renderMtree(mtreeEntries)
	if err != nil {
		return err
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
	pkgName := fmt.Sprintf("%s-%s-%s-%s.pkg.tar.zst",
		m.PKGBUILD.PkgName,
		m.PKGBUILD.PkgVer,
		m.PKGBUILD.PkgRel,
		m.PKGBUILD.ArchComputed)

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := shell.Exec(false, "",
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
	// Use centralized build dependencies
	buildDeps := constants.GetBuildDeps()
	installArgs := getBaseInstallArgs()
	installArgs = append(installArgs, buildDeps.Pacman...)

	if golang {
		platform.CheckGO()

		installArgs = append(installArgs, "go")
	}

	return shell.ExecWithSudo(false, "", "pacman", installArgs...)
}

// Update updates the Makepkg package manager.
//
// It retrieves the updates using the GetUpdates method of the PKGBUILD struct.
// It returns an error if there is any issue during the update process.
func (m *Pkg) Update() error {
	return m.PKGBUILD.GetUpdates("pacman", "-Sy")
}

func renderMtree(entries []*files.Entry) (string, error) {
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
			logger.Warn(i18n.T("logger.createmtreegzip.warn.failed_to_close_output_1"), "error", err)
		}
	}()

	// Create a gzip writer
	gzipWriter := pgzip.NewWriter(out)

	defer func() {
		err := gzipWriter.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createmtreegzip.warn.failed_to_close_gzip_1"), "error", err)
		}
	}()

	// Copy the source file to the gzip writer
	_, err = io.Copy(gzipWriter, strings.NewReader(mtreeContent))
	if err != nil {
		return err
	}

	return nil
}
