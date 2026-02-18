// Package files provides unified file system operations for package building.
package files

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// CheckWritable checks if a file is writable.
func CheckWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.files.failed_to_stat_file"), err)
	}

	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf(i18n.T("errors.files.file_not_writable"), path)
	}

	return nil
}

// Chmod changes file permissions.
func Chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		logger.Error(i18n.T("logger.chmod.error.failed_to_chmod_1"), "path", path, "error", err)
		return fmt.Errorf("%s: %w", i18n.T("errors.files.failed_to_change_permissions"), err)
	}

	return nil
}

// Create creates a new file.
func Create(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		logger.Error(i18n.T("logger.create.error.failed_to_create_path_1"), "path", path, "error", err)
		return nil, err
	}

	return file, nil
}

// CreateWrite creates a file and writes data to it.
func CreateWrite(path, data string) error {
	file, err := Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.files.warn.close_write"), "path", path, "error", closeErr)
		}
	}()

	if _, err = file.WriteString(data); err != nil {
		logger.Error(i18n.T("logger.createwrite.error.failed_to_write_to_1"), "path", path, "error", err)
		return err
	}

	return nil
}

// Exists checks if a file or directory exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// ExistsMakeDir creates a directory if it doesn't exist.
func ExistsMakeDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return fmt.Errorf(i18n.T("errors.files.failed_to_create_directory"), path)
		}
	} else if err != nil {
		return fmt.Errorf(i18n.T("errors.files.failed_to_access_directory"), path)
	}

	return nil
}

// GetDirSize calculates the total size of a directory.
func GetDirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return nil
	})

	return size, err
}

// GetFileType returns the ELF type of a file, or empty string for non-ELF files.
func GetFileType(path string) string {
	file, err := elf.Open(path)
	if err != nil {
		return ""
	}

	defer func() {
		_ = file.Close() // Ignore close error for this function
	}()

	switch file.Type {
	case elf.ET_EXEC:
		return "ET_EXEC"
	case elf.ET_DYN:
		return "ET_DYN"
	case elf.ET_REL:
		return "ET_REL"
	default:
		return "ET_NONE"
	}
}

// IsEmptyDir checks if a directory is empty.
func IsEmptyDir(path string, dirEntry os.DirEntry) bool {
	if !dirEntry.IsDir() {
		return false
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	return len(entries) == 0
}

// IsStaticLibrary checks if a file is a static library (.a file or archive).
func IsStaticLibrary(path string) bool {
	// Check by extension first
	if strings.HasSuffix(path, ".a") {
		return true
	}

	// Check by archive magic number
	file, err := os.Open(path) // #nosec G304 - path is validated by caller
	if err != nil {
		return false
	}

	defer func() {
		_ = file.Close() // Ignore close error for this function
	}()

	magic := make([]byte, 8)

	n, err := file.Read(magic)
	if err != nil || n < 8 {
		return false
	}

	return string(magic) == "!<arch>\n"
}

// Open opens a file for reading.
func Open(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		logger.Error(i18n.T("logger.open.error.failed_to_open_file_1"), "path", path, "error", err)
		return nil, err
	}

	return file, nil
}
