// Package files provides unified file system operations for package building.
package files

import (
	"crypto/sha256"
	"debug/elf"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Buffer pool for file operations to reduce memory allocations
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024) // 32KB buffer
	},
}

// CalculateSHA256 calculates the SHA256 hash of a file.
func CalculateSHA256(path string) ([]byte, error) {
	cleanFilePath := filepath.Clean(path)

	// Check if we can skip recalculation by comparing file modification time
	// with a cached result (this would require a cache implementation)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file for SHA256 calculation")
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("failed to close file", "path", cleanFilePath, "error", closeErr)
		}
	}()

	hash := sha256.New()

	_, err = io.Copy(hash, file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate SHA256")
	}

	return hash.Sum(nil), nil
}

// CheckWritable checks if a file is writable.
func CheckWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "failed to stat file for writability check")
	}

	if info.Mode().Perm()&0o200 == 0 {
		return errors.Errorf("file %s is not writable", path)
	}

	return nil
}

// Chmod changes file permissions.
func Chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		logger.Error("failed to chmod", "path", path, "error", err)
		return errors.Wrap(err, "failed to change file permissions")
	}

	return nil
}

// Create creates a new file.
func Create(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		logger.Error("failed to create path", "path", path, "error", err)
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
			logger.Warn("failed to close file", "path", path, "error", closeErr)
		}
	}()

	if _, err = file.WriteString(data); err != nil {
		logger.Error("failed to write to file", "path", path, "error", err)
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
			return errors.Errorf("failed to create directory %s", path)
		}
	} else if err != nil {
		return errors.Errorf("failed to access directory %s", path)
	}

	return nil
}

// Filename extracts the filename from a path.
func Filename(path string) string {
	n := strings.LastIndex(path, "/")
	if n == -1 {
		return path
	}

	return path[n+1:]
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
	if err != nil {
		return 0, errors.Wrap(err, "failed to calculate directory size")
	}

	return size, nil
}

// GetFileType returns the ELF file type as a string.
func GetFileType(path string) string {
	cleanFilePath := filepath.Clean(path)

	fileInfo, err := os.Lstat(cleanFilePath)
	if err != nil {
		logger.Debug("failed to get file info", "path", cleanFilePath, "error", err)
		return ""
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return ""
	}

	file, err := os.Open(cleanFilePath)
	if err != nil {
		logger.Debug("failed to open file for type detection", "path", cleanFilePath, "error", err)
		return ""
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("failed to close file", "path", cleanFilePath, "error", closeErr)
		}
	}()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return ""
	}

	return elfFile.Type.String()
}

// IsEmptyDir checks if a directory is empty.
func IsEmptyDir(path string, dirEntry os.DirEntry) bool {
	cleanFilePath := filepath.Clean(path)

	if !dirEntry.IsDir() {
		return false
	}

	entries, err := os.ReadDir(cleanFilePath)
	if err != nil {
		return false
	}

	return len(entries) == 0
}

// IsStaticLibrary checks if a file is a static library (.a file).
func IsStaticLibrary(path string) bool {
	if strings.HasSuffix(path, ".a") {
		return true
	}

	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return false
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("failed to close file", "path", cleanFilePath, "error", closeErr)
		}
	}()

	header := make([]byte, 8)

	_, err = file.Read(header)
	if err != nil {
		return false
	}

	return string(header) == "!<arch>\n"
}

// Open opens a file for reading.
func Open(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		logger.Error("failed to open file", "path", path, "error", err)
		return nil, err
	}

	return file, nil
}

// TryHardLink attempts to create a hard link instead of copying a file.
// Falls back to regular file copy if hard linking fails.
func TryHardLink(src, dst string) error {
	// Try to create a hard link first
	if err := os.Link(src, dst); err == nil {
		return nil
	}

	// Fall back to copying the file
	srcFile, err := os.Open(src) // #nosec G304 - src path is controlled by caller
	if err != nil {
		return errors.Wrap(err, "failed to open source file for copying")
	}

	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst) // #nosec G304 - dst path is controlled by caller
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}

	defer func() { _ = dstFile.Close() }()

	// Use buffer from pool for better performance
	buffer := bufferPool.Get().([]byte)
	defer bufferPool.Put(&buffer)

	if _, err := io.CopyBuffer(dstFile, srcFile, buffer); err != nil {
		return errors.Wrap(err, "failed to copy file content")
	}

	// Copy file permissions
	if srcInfo, err := srcFile.Stat(); err == nil {
		if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
			logger.Warn("failed to copy file permissions", "dst", dst, "error", err)
		}
	}

	return nil
}
