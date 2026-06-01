// Package platform provides system and platform detection utilities.
package platform

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	goArchivePath = "/tmp/go.tar.gz"
	goExecutable  = "/usr/bin/go"
	amd64Arch     = "amd64"
	i686Arch      = "386"
	armArch       = "arm"
	aarch64Arch   = "arm64"
	ppc64Arch     = "ppc64"
	ppc64leArch   = "ppc64le"
	s390xArch     = "s390x"
	mipsArch      = "mips"
	mipsleArch    = "mipsle"
	riscv64Arch   = "riscv64"
	armv7hArch    = "armv7h"
	ppc64Value    = "ppc64"
	ppc64leValue  = "ppc64le"
	s390xValue    = "s390x"
	mipsValue     = "mips"
	mipsleValue   = "mipsle"
	riscv64Value  = "riscv64"
)

// OSRelease represents operating system release information.
type OSRelease struct {
	ID       string
	Codename string // VERSION_CODENAME from /etc/os-release (e.g. "jammy" for Ubuntu 22.04)
}

// ParseOSRelease reads and parses the /etc/os-release file.
// It populates ID (e.g. "ubuntu") and Codename (e.g. "jammy") from the
// VERSION_CODENAME field so that callers can resolve distro-codename-specific
// PKGBUILD directives such as depends__ubuntu_jammy.
func ParseOSRelease() (OSRelease, error) {
	return parseOSReleaseFile("/etc/os-release")
}

// parseOSReleaseFile reads and parses the os-release file at the given path.
// Extracted from ParseOSRelease to allow testing with arbitrary files.
func parseOSReleaseFile(path string) (OSRelease, error) {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return OSRelease{}, errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.open_os_release_failed")).
			WithOperation("ParseOSRelease")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.platform.warn.failed_to_close_osrelease"), "error", closeErr)
		}
	}()

	var osRelease OSRelease

	scanner := bufio.NewScanner(file)

	fieldMap := map[string]*string{
		"ID":               &osRelease.ID,
		"VERSION_CODENAME": &osRelease.Codename,
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)

		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(parts[1], "\"")

			if fieldPtr, ok := fieldMap[key]; ok {
				*fieldPtr = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return OSRelease{}, errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.scan_os_release_failed")).
			WithOperation("ParseOSRelease")
	}

	return osRelease, nil
}

// IsPrivilegedHost reports whether the current process is running as
// uid 0 (root). The installers in pkg/aptinstall and pkg/apkindex
// use this as the heuristic for "we're inside a yap build container and
// it's safe to write to /, /var/lib/dpkg, /lib/apk/db, etc.".
//
// Non-root callers (developer workstations) hit the strict path and must
// either set RootDir / AllowRootInstall explicitly or run as root.
func IsPrivilegedHost() bool {
	return os.Geteuid() == 0
}

// GetArchitecture returns the system architecture mapped to package manager conventions.
func GetArchitecture() string {
	architectureMap := map[string]string{
		amd64Arch:   constants.ArchX86_64,
		i686Arch:    constants.ArchI686,
		armArch:     armv7hArch,
		aarch64Arch: constants.ArchAarch64,
		ppc64Arch:   ppc64Value,
		ppc64leArch: ppc64leValue,
		s390xArch:   s390xValue,
		mipsArch:    mipsValue,
		mipsleArch:  mipsleValue,
		riscv64Arch: riscv64Value,
	}

	currentArch := runtime.GOARCH

	pacmanArch, exists := architectureMap[currentArch]
	if !exists {
		logger.Warn(i18n.T("logger.platform.warn.arch_fallback"), "goarch", currentArch)
		return currentArch
	}

	return pacmanArch
}

// CheckGO checks if the Go compiler is installed and available.
func CheckGO() bool {
	_, err := os.Stat(goExecutable)
	if err == nil {
		logger.Info(i18n.T("logger.platform.info.go_is_already_installed"))
		return true
	}

	return false
}

// GOSetup installs and configures the Go compiler if not already present.
func GOSetup() error {
	if CheckGO() {
		return nil
	}

	_, err := shell.MultiPrinter.Start()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.platform.start_multiprinter_failed")).
			WithOperation("GOSetup")
	}

	if err := download.WithResumeContext(
		goArchivePath,
		constants.GoArchiveURL(),
		0,
		"yap", "go-toolchain",
		shell.MultiPrinter.Writer); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup")
	}

	if err := archive.Extract(context.Background(), goArchivePath, "/usr/lib"); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.platform.extract_go_archive_failed")).
			WithOperation("GOSetup")
	}

	if err := os.Symlink("/usr/lib/go/bin/go", goExecutable); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.create_go_symlink_failed")).
			WithOperation("GOSetup")
	}

	if err := os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt"); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.create_gofmt_symlink_failed")).
			WithOperation("GOSetup")
	}

	if err := os.RemoveAll(goArchivePath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.remove_go_archive_failed")).
			WithOperation("GOSetup")
	}

	logger.Info(i18n.T("logger.platform.info.go_successfully_installed"))

	return nil
}

// PullContainers downloads the specified container image for the given distribution.
func PullContainers(distro string) error {
	var containerApp string

	switch {
	case files.Exists("/usr/bin/podman"):
		containerApp = "/usr/bin/podman"
	case files.Exists("/usr/bin/docker"):
		containerApp = "/usr/bin/docker"
	default:
		return errors.New(errors.ErrTypeFileSystem,
			i18n.T("errors.platform.no_container_app_found"))
	}

	args := []string{
		"pull",
		constants.DockerOrg + distro,
	}

	if _, err := os.Stat(containerApp); err == nil {
		return shell.Exec(context.Background(), false, "", containerApp, args...)
	}

	return nil
}
