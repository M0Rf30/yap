package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

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

// MkdirAll creates a directory and all its parent directories.
//
// It takes a string parameter `path` which represents the path of the directory to be created.
// The function returns an error if any error occurs during the directory creation process.
func MkdirAll(path string) error {
	//#nosec
	err := os.MkdirAll(path, 0o755)
	if err != nil {
		Logger.Error("failed to make directory",
			Logger.Args("path", path))

		return err
	}

	return nil
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

// RemoveAll removes a file or directory and any children it contains.
//
// path: the path of the file or directory to be removed.
// error: an error if the removal fails.
func RemoveAll(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		Logger.Error("failed to remove",
			Logger.Args("path", path))

		return err
	}

	return nil
}
