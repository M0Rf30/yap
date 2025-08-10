package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

const (
	// Package type constants.
	packageTypeDeb = "deb"
	packageTypeRPM = "rpm"
	packageTypeApk = "apk"
	packageTypePkg = "pkg"
)

// installCmd represents the install command.
var installCmd = &cobra.Command{
	Use:     "install <artifact-file>",
	Short:   "📦 Install a package artifact using the appropriate package manager",
	GroupID: "utility",
	Long: `Install a package artifact by automatically detecting the package type
from the file extension and using the appropriate package manager.

SUPPORTED PACKAGE TYPES:
  • .deb     - Debian packages (apt-get)
  • .rpm     - RPM packages (dnf/yum)
  • .apk     - Alpine packages (apk)
  • .pkg.tar.* - Arch packages (pacman)

The command will automatically detect the package format and use the
appropriate system package manager to install the artifact with the
same arguments used by yap's internal package managers.`,
	Example: `  # Install a Debian package
  yap install /path/to/package.deb

  # Install an RPM package
  yap install /path/to/package.rpm

  # Install an Alpine package
  yap install /path/to/package.apk

  # Install an Arch package
  yap install /path/to/package.pkg.tar.zst`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

// runInstall handles the install command execution.
func runInstall(cmd *cobra.Command, args []string) error {
	artifactPath := args[0]

	// Check if file exists
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		return fmt.Errorf("artifact file not found: %s", artifactPath)
	}

	// Get absolute path
	absPath, err := filepath.Abs(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to resolve artifact path: %w", err)
	}

	// Detect package type from file extension
	packageType, err := detectPackageType(absPath)
	if err != nil {
		return err
	}

	logger.Info("detected package type",
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
		return "", fmt.Errorf("unsupported package format: %s\n"+
			"Supported formats: .deb, .rpm, .apk, .pkg.tar.*", fileName)
	}
}

// installPackage installs the package using the appropriate package manager
// with the same arguments used by yap's internal package managers.
func installPackage(packageType, artifactPath string) error {
	var cmd string

	var args []string

	switch packageType {
	case packageTypeDeb:
		// Use same args as pkg/dpkg/constants.go
		cmd = "apt-get"
		args = []string{"--allow-downgrades", "--assume-yes", "install", artifactPath}
	case packageTypeRPM:
		// Use same args as pkg/rpm/constants.go
		cmd = "dnf"
		args = []string{"-y", "install", artifactPath}
	case packageTypeApk:
		// Use same args as pkg/abuild/constants.go
		cmd = "apk"
		args = []string{"add", "--allow-untrusted", artifactPath}
	case packageTypePkg:
		// Use same args as used in pkg/makepkg/makepkg.go Install function
		cmd = "pacman"
		args = []string{"-U", "--noconfirm", artifactPath}
	default:
		return fmt.Errorf("unsupported package type: %s", packageType)
	}

	logger.Info("installing package",
		"command", cmd,
		"args", strings.Join(args, " "),
		"artifact", artifactPath)

	// Execute the installation command with the same pattern as internal managers
	err := osutils.Exec(false, "", cmd, args...)
	if err != nil {
		return fmt.Errorf("failed to install package with %s: %w", cmd, err)
	}

	logger.Info("package installed successfully",
		"artifact", artifactPath,
		"type", packageType)

	return nil
}

//nolint:gochecknoinits // Required for cobra command initialization
func init() {
	rootCmd.AddCommand(installCmd)
}
