// Package platform provides ownership preservation utilities for sudo environments.
package platform

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/pkg/errors"

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
func GetOriginalUser() (*OriginalUser, error) {
	sudoUser := os.Getenv("SUDO_USER")
	sudoUID := os.Getenv("SUDO_UID")
	sudoGID := os.Getenv("SUDO_GID")

	// If not running under sudo, return nil
	if sudoUser == "" || sudoUID == "" || sudoGID == "" {
		return nil, nil
	}

	uid, err := strconv.Atoi(sudoUID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse SUDO_UID")
	}

	gid, err := strconv.Atoi(sudoGID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse SUDO_GID")
	}

	// Get user info for additional details
	userInfo, err := user.Lookup(sudoUser)
	if err != nil {
		logger.Warn("failed to lookup sudo user info", "user", sudoUser, "error", err)
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
		return errors.Wrapf(err, "failed to chown %s to %s (%d:%d)", path, ou.Name, ou.UID, ou.GID)
	}

	logger.Debug("changed ownership to original user",
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
		return errors.Wrapf(err, "failed to chown root directory %s", path)
	}

	// Use filepath.Walk to recursively change ownership
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		err = syscall.Chown(walkPath, ou.UID, ou.GID)
		if err != nil {
			logger.Warn("failed to chown file",
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
		logger.Warn("failed to get original user info", "error", err)
		return nil // Don't fail the operation
	}

	if originalUser != nil {
		logger.Info("preserving ownership for original user",
			"path", path,

			"user", originalUser.Name)

		return originalUser.ChownToOriginalUser(path)
	}

	return nil
}

// PreserveOwnershipRecursive recursively changes ownership to original user if under sudo.
func PreserveOwnershipRecursive(path string) error {
	originalUser, err := GetOriginalUser()
	if err != nil {
		logger.Warn("failed to get original user info", "error", err)
		return nil // Don't fail the operation
	}

	if originalUser != nil {
		logger.Info("preserving ownership recursively for original user",

			"path", path,
			"user", originalUser.Name)

		return originalUser.ChownRecursiveToOriginalUser(path)
	}

	return nil
}
