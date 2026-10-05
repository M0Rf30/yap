// Package platform provides system and platform detection utilities.
package platform

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	goArchiveName      = "go.tar.gz"
	goChecksumMaxBytes = 1024
	goExecutable       = "/usr/bin/go"
	amd64Arch          = "amd64"
	i686Arch           = "386"
	armArch            = "arm"
	aarch64Arch        = "arm64"
	ppc64Arch          = "ppc64"
	ppc64leArch        = "ppc64le"
	s390xArch          = "s390x"
	mipsArch           = "mips"
	mipsleArch         = "mipsle"
	riscv64Arch        = "riscv64"
	armv7hArch         = "armv7h"
	ppc64Value         = "ppc64"
	ppc64leValue       = "ppc64le"
	s390xValue         = "s390x"
	mipsValue          = "mips"
	mipsleValue        = "mipsle"
	riscv64Value       = "riscv64"
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
//
// The tarball is downloaded into a private (0700) temporary directory and
// verified against the SHA-256 published next to it on go.dev before being
// extracted as root, so a pre-planted or truncated file can never be installed.
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

	tmpDir, err := os.MkdirTemp("", "yap-go-")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup")
	}

	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			logger.Warn(i18n.T("errors.platform.remove_go_archive_failed"),
				"path", tmpDir,
				"error", rmErr)
		}
	}()

	archivePath := filepath.Join(tmpDir, goArchiveName)
	archiveURL := constants.GoArchiveURL()

	if err := download.WithResumeContext(
		archivePath,
		archiveURL,
		download.MaxRetries(),
		"yap", "go-toolchain",
		shell.MultiPrinter.Writer); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup")
	}

	if err := verifyGoArchive(context.Background(), archivePath, archiveURL); err != nil {
		return err
	}

	if err := archive.Extract(context.Background(), archivePath, "/usr/lib"); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.platform.extract_go_archive_failed")).
			WithOperation("GOSetup")
	}

	if err := ensureSymlink("/usr/lib/go/bin/go", goExecutable); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.create_go_symlink_failed")).
			WithOperation("GOSetup")
	}

	if err := ensureSymlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt"); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.create_gofmt_symlink_failed")).
			WithOperation("GOSetup")
	}

	logger.Info(i18n.T("logger.platform.info.go_successfully_installed"))

	return nil
}

// verifyGoArchive checks the downloaded archive against the SHA-256 published
// at <archiveURL>.sha256.
func verifyGoArchive(ctx context.Context, archivePath, archiveURL string) error {
	body, err := httpclient.FetchBytes(ctx, archiveURL+".sha256", goChecksumMaxBytes)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("reason", "fetching SHA-256 checksum failed").
			WithContext("url", archiveURL+".sha256")
	}

	expected, err := parseSHA256Sidecar(body)
	if err != nil {
		return err
	}

	return verifyFileSHA256(archivePath, expected)
}

// parseSHA256Sidecar extracts the hex digest from a .sha256 sidecar file, which
// holds either the bare digest or "<digest>  <filename>".
func parseSHA256Sidecar(body []byte) (string, error) {
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", errors.New(errors.ErrTypeValidation,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("reason", "empty SHA-256 checksum")
	}

	digest := strings.ToLower(fields[0])

	raw, err := hex.DecodeString(digest)
	if err != nil || len(raw) != sha256.Size {
		return "", errors.New(errors.ErrTypeValidation,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("reason", "malformed SHA-256 checksum")
	}

	return digest, nil
}

// verifyFileSHA256 returns an error unless path hashes to the expected hex digest.
func verifyFileSHA256(path, expected string) error {
	f, err := os.Open(path) //nolint:gosec // path lives in a private temp dir
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("path", path)
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("path", path)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return errors.New(errors.ErrTypeValidation,
			i18n.T("errors.platform.download_go_archive_failed")).
			WithOperation("GOSetup").
			WithContext("reason", "SHA-256 checksum mismatch").
			WithContext("expected", expected).
			WithContext("actual", actual)
	}

	return nil
}

// ensureSymlink makes link a symlink to target. An existing symlink is replaced
// so repeated installs are idempotent; a non-symlink file is left untouched.
func ensureSymlink(target, link string) error {
	info, err := os.Lstat(link)

	switch {
	case err == nil && info.Mode()&os.ModeSymlink == 0:
		logger.Debug(i18n.T("logger.platform.debug.not_replacing_existing_file"), "path", link)

		return nil
	case err == nil:
		if cur, rlErr := os.Readlink(link); rlErr == nil && cur == target {
			return nil
		}

		if rmErr := os.Remove(link); rmErr != nil {
			return rmErr //nolint:wrapcheck // wrapped by caller
		}
	case !os.IsNotExist(err):
		return err //nolint:wrapcheck // wrapped by caller
	}

	return os.Symlink(target, link) //nolint:wrapcheck // wrapped by caller
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
