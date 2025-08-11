// Package platform provides system and platform detection utilities.
package platform

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/files"
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
		return OSRelease{}, errors.Wrap(err, "failed to open /etc/os-release")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("failed to close os-release file", "error", closeErr)
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
		return OSRelease{}, errors.Wrap(err, "failed to scan os-release file")
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
		logger.Warn("unknown architecture, falling back to GOARCH", "goarch", currentArch)
		return currentArch
	}

	return pacmanArch
}

// CheckGO checks if the Go compiler is installed and available.
func CheckGO() bool {
	_, err := os.Stat(goExecutable)
	if err == nil {
		logger.Info("go is already installed")
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
		return errors.Wrap(err, "failed to start multiprinter")
	}

	if err := download.Download(
		goArchivePath,
		constants.GoArchiveURL,
		shell.MultiPrinter.Writer); err != nil {
		return errors.Wrap(err, "failed to download Go archive")
	}

	if err := archive.Extract(goArchivePath, "/usr/lib"); err != nil {
		return errors.Wrap(err, "failed to extract Go archive")
	}

	if err := os.Symlink("/usr/lib/go/bin/go", goExecutable); err != nil {
		return errors.Wrap(err, "failed to create go symlink")
	}

	if err := os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt"); err != nil {
		return errors.Wrap(err, "failed to create gofmt symlink")
	}

	if err := os.RemoveAll(goArchivePath); err != nil {
		return errors.Wrap(err, "failed to remove go archive")
	}

	logger.Info("go successfully installed")

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
		return errors.New("no container application found")
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
