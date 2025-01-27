package utils

import (
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// Symbolic link.
	TagLink = 0o120000
	// Directory.
	TagDirectory = 0o40000
	// TypeFile is the type of a regular file. This is also the type that is
	// implied when no type is specified.
	TypeFile = "file"
	// TypeDir is the type of a directory that is explicitly added in order to
	// declare ownership or non-standard permission.
	TypeDir = "dir"
	// TypeSymlink is the type of a symlink that is created at the destination
	// path and points to the source path.
	TypeSymlink = "symlink"
	// TypeConfig is the type of a configuration file that may be changed by the
	// user of the package.
	TypeConfig = "config"
	// TypeConfigNoReplace is like TypeConfig with an added noreplace directive
	// that is respected by RPM-based distributions.
	// For all other package formats it is handled exactly like TypeConfig.
	TypeConfigNoReplace = "config|noreplace"
)

// FileContent describes the source and destination
// of one file to copy into a package.
type FileContent struct {
	Source      string
	Destination string
	Type        string
	FileInfo    *FileInfo
}

type FileInfo struct {
	Owner string
	Group string
	Mode  os.FileMode
	MTime time.Time
	Size  int64
}

// CheckWritable checks if a binary file is writeable.
//
// It checks if the file exists and if write permission is granted.
// If the file does not exist or does not have write permission,
// an error is returned.
func CheckWritable(binary string) error {
	info, err := os.Stat(binary)
	if err != nil || info.Mode().Perm()&0200 == 0 {
		return err
	}

	return nil
}

// Chmod changes the file mode of the specified path.
//
// It takes a string parameter `path` which represents the path of the file
// whose mode needs to be changed. It also takes an `os.FileMode` parameter
// `perm` which represents the new file mode to be set.
//
// The function returns an error if there was an issue changing the file
// mode. If the file mode was successfully changed, it returns nil.
func Chmod(path string, perm os.FileMode) error {
	err := os.Chmod(path, perm)
	if err != nil {
		Logger.Error("failed to chmod", Logger.Args("path", path))

		return err
	}

	return nil
}

// Create creates a new file at the specified path.
//
// It takes a string parameter `path` which represents the path of the file to be created.
// The function returns a pointer to an `os.File` and an error.
func Create(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Create(cleanFilePath)

	if err != nil {
		Logger.Error("failed to create path", Logger.Args("path", path))
	}

	return file, err
}

// CreateWrite writes the given data to the file specified by the path.
//
// It takes two parameters:
// - path: a string representing the path of the file.
// - data: a string representing the data to be written to the file.
//
// It returns an error if there was a problem creating or writing to the file.
func CreateWrite(path, data string) error {
	file, err := Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(data)
	if err != nil {
		Logger.Error("failed to write to file", Logger.Args("path", path))

		return err
	}

	return nil
}

// Exists checks if a file or directory exists at the given path.
//
// path: the path to the file or directory.
// bool: returns true if the file or directory exists, false otherwise.
func Exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil || !os.IsNotExist(err)
}

// ExistsMakeDir checks if a directory exists at the given path and creates it if it doesn't.
//
// path: the path to the directory.
// error: returns an error if the directory cannot be created or accessed.
func ExistsMakeDir(path string) error {
	// #nosec
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return errors.Errorf("failed to create directory %s", path)
		}
	} else if err != nil {
		return errors.Errorf("failed to access directory %s", path)
	}

	return nil
}

// Filename returns the filename from a given path.
//
// It takes a string parameter `path` which represents the path of the file.
// It returns a string which is the filename extracted from the path.
func Filename(path string) string {
	n := strings.LastIndex(path, "/")
	if n == -1 {
		return path
	}

	return path[n+1:]
}

// GetDirSize calculates the size of a directory in kilobytes.
//
// It takes a path as a parameter and returns the size of the directory in kilobytes and an error if any.
func GetDirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			Logger.Fatal("failed to get dir size",
				Logger.Args("path", path))
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return err
	})

	size /= 1024

	return size, err
}

// GetFileType uses readelf to determine the type of the binary file.
func GetFileType(binary string) string {
	cleanFilePath := filepath.Clean(binary)

	// Check if the file is a symlink
	fileInfo, err := os.Lstat(cleanFilePath)
	if err != nil {
		Logger.Fatal("fatal error", Logger.Args("error", err))
	}

	// Skip if it's a symbolic link
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return ""
	}

	// Open the ELF binary
	file, err := os.Open(cleanFilePath)
	if err != nil {
		Logger.Fatal("fatal error",
			Logger.Args("error", err))
	}
	defer file.Close()

	// Parse the ELF file
	elfFile, err := elf.NewFile(file)
	if err != nil {
		return ""
	}

	return elfFile.Type.String()
}

// IsEmptyDir checks if a directory is empty.
//
// It takes in two parameters: path, a string representing the directory path,
// and dirEntry, an os.DirEntry representing the directory entry. It returns a
// boolean value indicating whether the directory is empty or not.
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

// isStaticLibrary checks if the binary is a static library.
func IsStaticLibrary(binary string) bool {
	// Check the file extension
	if strings.HasSuffix(binary, ".a") {
		return true
	}

	cleanFilePath := filepath.Clean(binary)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Read the first few bytes to check the format
	header := make([]byte, 8)
	_, err = file.Read(header)

	if err != nil {
		return false
	}

	// Check for the "!<arch>" magic string which indicates a static library
	if string(header) == "!<arch>\n" {
		return true
	}

	return false
}

// Open opens a file at the specified path and returns a pointer to the file and an error.
//
// The path parameter is a string representing the file path to be opened.
// The function returns a pointer to an os.File and an error.
func Open(path string) (*os.File, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		Logger.Error("failed to open file",
			Logger.Args("path", path))
	}

	return file, err
}

// StripFile strips the binary file using the strip command.
func StripFile(binary string, args ...string) error {
	return strip(binary, args...)
}

// StripLTO strips LTO-related sections from the binary file.
func StripLTO(binary string, args ...string) error {
	return strip(binary, append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

// strip performs a strip operation on the specified binary file.
//
// The strip command removes any symbol table from the binary executable.
// This can be useful for smaller binary sizes, but makes debugging and
// analysis more difficult.
//
// The original file is replaced with the stripped one.
func strip(binary string, args ...string) error {
	tempFile, err := os.CreateTemp("", filepath.Base(binary))
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name()) // Ensure the temp file is removed

	args = append(args, "-o", tempFile.Name(), binary)

	if err := Exec(false, "", "strip", args...); err != nil {
		return err
	}

	if err := os.Rename(tempFile.Name(), binary); err != nil {
		return err
	}

	return nil
}
