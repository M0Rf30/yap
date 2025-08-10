// Package system provides basic file system operations and utilities.
package system

import (
	"crypto/sha256"
	"debug/elf"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	// TagLink represents a symbolic link.
	TagLink = 0o120000
	// TagDirectory represents a directory.
	TagDirectory = 0o40000
	// TypeFile is the type of a regular file. This is also the type that is
	// implied when no type is specified.
	TypeFile = "file"
	// TypeDir is the type of a directory that is explicitly added in order to
	// declare ownership or non-standard permission.
	TypeDir = "dir"
	// TypeImplicitDir is the type of a directory that is implicitly added as a
	// parent of a file.
	TypeImplicitDir = "implicit dir"
	// TypeSymlink is the type of a symlink that is created at the destination
	// path and points to the source path.
	TypeSymlink = "symlink"
	// TypeTree is the type of a whole directory tree structure.
	TypeTree = "tree"
	// TypeConfig is the type of a configuration file that may be changed by the
	// user of the package.
	TypeConfig = "config"
	// TypeConfigNoReplace is like TypeConfig with an added noreplace directive
	// that is respected by RPM-based distributions.
	// For all other package formats it is handled exactly like TypeConfig.
	TypeConfigNoReplace = "config|noreplace"
)

// FileInfo contains file metadata for package operations.
type FileInfo struct {
	Mode    uint32
	ModTime int64
	Size    int64
}

// CalculateSHA256 calculates the SHA-256 checksum of a file.
func CalculateSHA256(path string) ([]byte, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = file.Close() // Ignore close errors for simplicity
	}()

	hash := sha256.New()

	_, err = io.Copy(hash, file)
	if err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

// CheckWritable checks if a binary file is writeable.
func CheckWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o200 == 0 {
		return err
	}

	return nil
}

// Chmod changes the file mode of the specified path.
func Chmod(path string, perm os.FileMode) error {
	return os.Chmod(path, perm)
}

// Create creates a new file at the specified path.
func Create(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)
	return os.Create(cleanFilePath)
}

// CreateWrite writes the given data to the file specified by the path.
func CreateWrite(path, data string) error {
	file, err := Create(path)
	if err != nil {
		return err
	}

	defer func() {
		_ = file.Close() // Ignore close errors for simplicity
	}()

	_, err = file.WriteString(data)

	return err
}

// Exists checks if a file or directory exists at the given path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// ExistsMakeDir checks if a directory exists at the given path and creates it if it doesn't.
func ExistsMakeDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err := os.MkdirAll(path, os.ModePerm) // #nosec
		if err != nil {
			return errors.Errorf("failed to create directory %s", path)
		}
	} else if err != nil {
		return errors.Errorf("failed to access directory %s", path)
	}

	return nil
}

// Filename returns the filename from a given path.
func Filename(path string) string {
	n := strings.LastIndex(path, "/")
	if n == -1 {
		return path
	}

	return path[n+1:]
}

// GetDirSize calculates the size of a directory in bytes.
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

// GetFileType uses readelf to determine the type of the binary file.
func GetFileType(path string) string {
	cleanFilePath := filepath.Clean(path)

	// Check if the file is a symlink
	fileInfo, err := os.Lstat(cleanFilePath)
	if err != nil {
		return ""
	}

	// Skip if it's a symbolic link
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return ""
	}

	// Open the ELF binary
	file, err := os.Open(cleanFilePath)
	if err != nil {
		return ""
	}

	defer func() {
		_ = file.Close() // Ignore close errors for simplicity
	}()

	// Parse the ELF file
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

// IsStaticLibrary checks if the binary is a static library.
func IsStaticLibrary(path string) bool {
	// Check the file extension
	if strings.HasSuffix(path, ".a") {
		return true
	}

	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return false
	}

	defer func() {
		_ = file.Close() // Ignore close errors for simplicity
	}()

	// Read the first few bytes to check the format
	header := make([]byte, 8)

	_, err = file.Read(header)
	if err != nil {
		return false
	}

	// Check for the "!<arch>" magic string which indicates a static library
	return string(header) == "!<arch>\n"
}

// Open opens a file at the specified path and returns a pointer to the file and an error.
func Open(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)
	return os.Open(cleanFilePath)
}
