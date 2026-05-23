package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/dnfinstall"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	// Package type constants.
	packageTypeDeb = "deb"
	packageTypeRPM = "rpm"
	packageTypeApk = "apk" // Also defined in constants.go for reuse
	packageTypePkg = "pkg"
)

// installCmd represents the install command.
var installCmd = &cobra.Command{
	Use:     commandInstall + " <artifact-file>",
	Short:   "", // Set by InitializeLocalizedDescriptions
	GroupID: commandUtility,
	Long:    "", // Will be set in init()
	Example: "", // Will be set in init()
	Args:    cobra.ExactArgs(1),
	RunE:    runInstall,
}

// runInstall handles the install command execution.
func runInstall(cmd *cobra.Command, args []string) error {
	artifactPath := args[0]

	// Check if file exists
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		return errors.New(errors.ErrTypeFileSystem,
			fmt.Sprintf(i18n.T("errors.install.artifact_not_found"), artifactPath)).
			WithOperation("runInstall").
			WithContext("path", artifactPath)
	}

	// Get absolute path
	absPath, err := filepath.Abs(artifactPath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.install.failed_to_resolve_path")).
			WithOperation("runInstall").
			WithContext("path", artifactPath)
	}

	// Detect package type from file extension
	packageType, err := detectPackageType(absPath)
	if err != nil {
		return err
	}

	logger.Info(i18n.T("logger.runinstall.info.detected_package_type_1"),
		"artifact", absPath,
		"type", packageType)

	// Install using appropriate package manager
	return installPackage(packageType, absPath)
}

// detectPackageType determines the package type from file extension.
func detectPackageType(filePath string) (string, error) {
	fileName := filepath.Base(filePath)
	lowerName := strings.ToLower(fileName)

	switch {
	case strings.HasSuffix(lowerName, ".deb"):
		return packageTypeDeb, nil
	case strings.HasSuffix(lowerName, ".rpm"):
		return packageTypeRPM, nil
	case strings.HasSuffix(lowerName, ".apk"):
		return packageTypeApk, nil
	case strings.Contains(lowerName, ".pkg.tar."):
		return packageTypePkg, nil
	default:
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.install.unsupported_package_format"), fileName)).
			WithOperation("detectPackageType").
			WithContext("fileName", fileName)
	}
}

// installPackage installs the package using the appropriate package manager
// with the same arguments used by yap's internal package managers.
func installPackage(packageType, artifactPath string) error {
	// RPM: use pure-Go dnfinstall
	if packageType == packageTypeRPM {
		logger.Info(i18n.T("logger.installpackage.info.installing_package_1"),
			"command", "dnfinstall",
			"artifact", artifactPath)

		opts := dnfinstall.Options{
			RootDir:             "/",
			AllowRootInstall:    true,
			AllowUnverifiedRPMs: false, // require GPG-trusted signature
			RunLDConfig:         true,
		}

		if err := dnfinstall.InstallFile(context.Background(), artifactPath, opts); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild,
				i18n.T("errors.install.installation_failed")).
				WithOperation("installPackage").
				WithContext("artifact", artifactPath).
				WithContext("packageType", packageType)
		}

		logger.Info(i18n.T("logger.installpackage.info.package_installed_successfully_1"),
			"artifact", artifactPath,
			"type", packageType)

		return nil
	}

	// Other formats: use subprocess-based package managers
	var cmd string
	var args []string

	switch packageType {
	case packageTypeDeb:
		// Use same args as pkg/dpkg/constants.go
		cmd = "apt-get"
		args = []string{"--allow-downgrades", "--assume-yes", "install", artifactPath}
	case packageTypeApk:
		// Use same args as pkg/abuild/constants.go
		cmd = pmApk
		args = []string{"add", "--allow-untrusted", artifactPath}
	case packageTypePkg:
		// Use same args as used in pkg/makepkg/makepkg.go Install function
		cmd = "pacman"
		args = []string{"-U", "--noconfirm", artifactPath}
	default:
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.install.unsupported_package_type"), packageType)).
			WithOperation("installPackage").
			WithContext("packageType", packageType)
	}

	logger.Info(i18n.T("logger.installpackage.info.installing_package_1"),
		"command", cmd,
		"args", strings.Join(args, " "),
		"artifact", artifactPath)

	// Execute the installation command with the same pattern as internal managers
	err := shell.Exec(context.Background(), false, "", cmd, args...)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.install.installation_failed")).
			WithOperation("installPackage").
			WithContext("command", cmd).
			WithContext("packageType", packageType)
	}

	logger.Info(i18n.T("logger.installpackage.info.package_installed_successfully_1"),
		"artifact", artifactPath,
		"type", packageType)

	return nil
}

// InitializeInstallDescriptions sets the localized descriptions for the install command.
// This must be called after i18n is initialized.
func InitializeInstallDescriptions() {
	initCommandDescriptions(installCmd, "install", map[string]string{})
}

//nolint:gochecknoinits // Required for cobra command initialization
func init() {
	rootCmd.AddCommand(installCmd)
}
