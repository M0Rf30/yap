package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/M0Rf30/yap/constants"
)

func MkdirAll(path string) error {
	err := os.MkdirAll(path, 0o755)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to mkdir '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func Chmod(path string, perm os.FileMode) error {
	err := os.Chmod(path, perm)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to chmod '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func ChownR(path string, user, group string) error {
	err := Exec("",
		"chown",
		"-R",
		fmt.Sprintf("%s:%s", user, group),
		path,
	)

	if err != nil {
		return err
	}

	return err
}

func Remove(path string) error {
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to remove '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func RemoveAll(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to remove '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func ExistsMakeDir(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = MkdirAll(path)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("%s❌ :: %sfailed to stat '%s'%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				path,
				string(constants.ColorWhite))

			return err
		}

		return err
	}

	return err
}

func Create(path string) (*os.File, error) {
	file, err := os.Create(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to create '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))
	}

	return file, err
}

func CreateWrite(path string, data string) error {
	file, err := Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(data)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to write to file '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open file '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))
	}

	return file, err
}

// CopyFile copies the contents of the file named source to the file named
// by dest. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file. The file mode will be copied from the source and
// the copied data is synced/flushed to stable storage.
func CopyFile(source, dest string) (err error) {
	file, err := os.Open(source)
	if err != nil {
		return
	}
	defer file.Close()

	out, err := os.Create(dest)
	if err != nil {
		return
	}

	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, file)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	si, err := os.Stat(source)
	if err != nil {
		return
	}

	err = os.Chmod(dest, si.Mode())
	if err != nil {
		return
	}

	return err
}

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
// Symlinks are ignored and skipped.
func CopyDir(source string, dest string) error {
	source = filepath.Clean(source)
	dest = filepath.Clean(dest)

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	_, err = os.Stat(dest)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	err = os.MkdirAll(dest, sourceInfo.Mode())
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to read dir '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			source,
			string(constants.ColorWhite))

		return err
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			err = CopyDir(sourcePath, destPath)
			if err != nil {
				return err
			}
		} else {
			err = CopyFile(sourcePath, destPath)
			if err != nil {
				return err
			}
		}
	}

	return err
}

func FindExt(path string, extension string) ([]string, error) {
	var files []string

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err)

			return err
		}

		if !info.IsDir() && filepath.Ext(path) == extension {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	return files, err
}

func Filename(path string) string {
	n := strings.LastIndex(path, "/")
	if n == -1 {
		return path
	}

	return path[n+1:]
}

func GetDirSize(path string) int {
	output, err := ExecOutput("", "du", "-c", "-s", path)
	if err != nil {
		os.Exit(1)
	}

	split := strings.Fields(output)

	size, err := strconv.Atoi(split[len(split)-2])
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to get dir size '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return size
	}

	return size
}

func Exists(path string) (bool, error) {
	exists := false
	_, err := os.Stat(path)

	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		} else {
			fmt.Printf("utils: Exists check error for '%s'\n", path)
			log.Fatal(err)

			return exists, err
		}
	} else {
		exists = true
	}

	return exists, err
}
