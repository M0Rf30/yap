// Package platform provides ownership preservation utilities for sudo environments.
package platform

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// OriginalUser holds information about the original user when running under sudo.
type OriginalUser struct {
	UID  int
	GID  int
	Name string
	Home string
}

// GetOriginalUser detects if yap is running under sudo and returns original user info.
// Returns (nil, nil) if not running under sudo - this is not an error condition.
//
//nolint:nilnil // Returning (nil, nil) is intentional when not under sudo
func GetOriginalUser() (*OriginalUser, error) {
	sudoUser := os.Getenv("SUDO_USER")
	sudoUID := os.Getenv("SUDO_UID")
	sudoGID := os.Getenv("SUDO_GID")

	// If not running under sudo, return nil - this is not an error
	if sudoUser == "" || sudoUID == "" || sudoGID == "" {
		return nil, nil
	}

	uid, err := strconv.Atoi(sudoUID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			i18n.T("errors.platform.parse_sudo_uid_failed")).
			WithOperation("GetOriginalUser")
	}

	gid, err := strconv.Atoi(sudoGID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			i18n.T("errors.platform.parse_sudo_gid_failed")).
			WithOperation("GetOriginalUser")
	}

	// Get user info for additional details
	userInfo, err := user.Lookup(sudoUser)
	if err != nil {
		logger.Warn(i18n.T("logger.platform.warn.sudo_lookup"), "user", sudoUser, "error", err)
		// Continue with basic info
		return &OriginalUser{
			UID:  uid,
			GID:  gid,
			Name: sudoUser,
			Home: "",
		}, nil
	}

	return &OriginalUser{
		UID:  uid,
		GID:  gid,
		Name: sudoUser,
		Home: userInfo.HomeDir,
	}, nil
}

// IsRunningSudo returns true if yap is currently running under sudo.
func IsRunningSudo() bool {
	originalUser, _ := GetOriginalUser()
	return originalUser != nil
}

// ChownToOriginalUser changes ownership of the given path to the original user.
func (ou *OriginalUser) ChownToOriginalUser(path string) error {
	if ou == nil {
		return nil // No original user, nothing to do
	}

	err := os.Chown(path, ou.UID, ou.GID)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.chown_failed")).
			WithOperation("ChownToOriginalUser").
			WithContext("path", path).
			WithContext("user", ou.Name)
	}

	logger.Debug(i18n.T("logger.platform.debug.changed_ownership_to_original"),
		"path", path,
		"user", ou.Name,
		"uid", ou.UID,
		"gid", ou.GID)

	return nil
}

// ChownRecursiveToOriginalUser recursively changes ownership of directory to original user.
func (ou *OriginalUser) ChownRecursiveToOriginalUser(path string) error {
	if ou == nil {
		return nil // No original user, nothing to do
	}

	err := syscall.Chown(path, ou.UID, ou.GID)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.platform.chown_root_directory_failed")).
			WithOperation("ChownRecursiveToOriginalUser").
			WithContext("path", path)
	}

	// Use filepath.Walk to recursively change ownership
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		err = syscall.Chown(walkPath, ou.UID, ou.GID)
		if err != nil {
			logger.Warn(i18n.T("logger.platform.warn.failed_to_chown_file"),
				"path", walkPath,
				"user", ou.Name,
				"error", err)
			// Continue with other files rather than failing completely
			return nil
		}

		return nil
	})
}

// PreserveOwnership changes ownership to original user if running under sudo.
func PreserveOwnership(path string) error {
	originalUser, err := GetOriginalUser()
	if err != nil {
		logger.Warn(i18n.T("logger.common.warn.failed_to_get_original"), "error", err)
		return nil // Don't fail the operation
	}

	if originalUser != nil {
		logger.Info(i18n.T("logger.platform.info.preserving_ownership_for_original"),
			"path", path,

			"user", originalUser.Name)

		return originalUser.ChownToOriginalUser(path)
	}

	return nil
}

// PreserveOwnershipRecursive recursively changes ownership to original user if under sudo.
//
// In container environments (detected via YAP_IN_CONTAINER=1, baked into all
// yap Docker images by build/deploy/generate.sh) the chown is skipped. The
// container's runtime user is already the intended build user, so chowning the
// source tree adds nothing and on overlayfs can trigger copy-up races that
// cause execve() of freshly-chowned scripts to return ENOEXEC (observed in
// k3s/Fedora CoreOS Jenkins pods).
func PreserveOwnershipRecursive(path string) error {
	if os.Getenv("YAP_IN_CONTAINER") == "1" {
		logger.Debug(i18n.T("logger.platform.debug.skipping_recursive_ownership_preservation"), "path", path)

		return nil
	}

	originalUser, err := GetOriginalUser()
	if err != nil {
		logger.Warn(i18n.T("logger.platform.warn.get_user"), "error", err)
		return nil // Don't fail the operation
	}

	if originalUser != nil {
		logger.Info(i18n.T("logger.platform.info.preserve_recursive"),

			"path", path,
			"user", originalUser.Name)

		return originalUser.ChownRecursiveToOriginalUser(path)
	}

	return nil
}
