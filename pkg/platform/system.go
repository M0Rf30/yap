// Package platform provides system and platform detection utilities.
package platform

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	goArchivePath = "/tmp/go.tar.gz"
	goExecutable  = "/usr/bin/go"
)

// OSRelease represents operating system release information.
type OSRelease struct {
	ID string
}

// ParseOSRelease reads and parses the /etc/os-release file.
func ParseOSRelease() (OSRelease, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return OSRelease{}, fmt.Errorf("%s: %w", i18n.T("errors.platform.open_os_release_failed"), err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.parseosrelease.warn.failed_to_close_osrelease_1"), "error", closeErr)
		}
	}()

	var osRelease OSRelease

	scanner := bufio.NewScanner(file)

	fieldMap := map[string]*string{
		"ID": &osRelease.ID,
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
		return OSRelease{}, fmt.Errorf("%s: %w", i18n.T("errors.platform.scan_os_release_failed"), err)
	}

	return osRelease, nil
}

// GetArchitecture returns the system architecture mapped to package manager conventions.
func GetArchitecture() string {
	architectureMap := map[string]string{
		"amd64":   "x86_64",
		"386":     "i686",
		"arm":     "armv7h",
		"arm64":   "aarch64",
		"ppc64":   "ppc64",
		"ppc64le": "ppc64le",
		"s390x":   "s390x",
		"mips":    "mips",
		"mipsle":  "mipsle",
		"riscv64": "riscv64",
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
		logger.Info(i18n.T("logger.checkgo.info.go_is_already_installed_1"))
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
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.start_multiprinter_failed"), err)
	}

	if err := download.Download(
		goArchivePath,
		constants.GoArchiveURL,
		shell.MultiPrinter.Writer); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.download_go_archive_failed"), err)
	}

	if err := archive.Extract(goArchivePath, "/usr/lib"); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.extract_go_archive_failed"), err)
	}

	if err := os.Symlink("/usr/lib/go/bin/go", goExecutable); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.create_go_symlink_failed"), err)
	}

	if err := os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt"); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.create_gofmt_symlink_failed"), err)
	}

	if err := os.RemoveAll(goArchivePath); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.platform.remove_go_archive_failed"), err)
	}

	logger.Info(i18n.T("logger.gosetup.info.go_successfully_installed_1"))

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
		return errors.New(i18n.T("errors.platform.no_container_app_found"))
	}

	args := []string{
		"pull",
		constants.DockerOrg + distro,
	}

	if _, err := os.Stat(containerApp); err == nil {
		return shell.Exec(false, "", containerApp, args...)
	}

	return nil
}
