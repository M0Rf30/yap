package options

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/osutils"
)

// Strip walks through the directory to process each file.
func Strip(packageDir string) error {
	osutils.Logger.Info("stripping binaries")

	return filepath.WalkDir(packageDir, processFile)
}

// processFile Processes a single file, checking for stripping and LTO
// operations if applicable.
func processFile(binary string, dirEntry fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	if dirEntry.IsDir() {
		return nil
	}

	// Always try to ensure the file is writable before stripping
	osutils.Logger.Debug("ensuring file is writable before stripping", osutils.Logger.Args("file", binary))

	info, err := os.Stat(binary)
	if err != nil {
		osutils.Logger.Warn("failed to get file info", osutils.Logger.Args("file", binary, "error", err))

		return nil
	}

	chmodErr := osutils.Chmod(binary, info.Mode().Perm()|0o200)
	if chmodErr != nil {
		osutils.Logger.Warn("failed to make file writable", osutils.Logger.Args("file", binary, "error", chmodErr))

		return nil // Skip if we can't change permissions
	}

	err = osutils.CheckWritable(binary)
	if err != nil {
		osutils.Logger.Warn("file still not writable after chmod", osutils.Logger.Args("file", binary, "error", err))

		return nil // Skip if not writable
	}

	fileType := osutils.GetFileType(binary)
	if fileType == "" || fileType == "ET_NONE" {
		return err
	}

	stripFlags, stripLTO := determineStripFlags(fileType, binary)

	osutils.Logger.Debug("about to strip binary", osutils.Logger.Args("file", binary, "flags", stripFlags))

	err = osutils.StripFile(binary, stripFlags)
	if err != nil {
		osutils.Logger.Error("strip command failed", osutils.Logger.Args("file", binary, "flags", stripFlags, "error", err))

		return err
	}

	if stripLTO {
		err := osutils.StripLTO(binary)
		if err != nil {
			return err
		}
	}

	return nil
}

// determineStripFlags determines strip flags based on file
// type and binary.
//
// Returns strip flags and whether the binary is a shared library.
func determineStripFlags(fileType, binary string) (string, bool) {
	stripBinaries := "--strip-all"
	stripShared := "--strip-unneeded"
	stripStatic := "--strip-debug"

	switch {
	case strings.Contains(fileType, "ET_DYN"):
		return stripShared, false
	case strings.Contains(fileType, "ET_EXEC"):
		return stripBinaries, false
	case strings.Contains(fileType, "ET_REL"):
		isStatic := osutils.IsStaticLibrary(binary)
		if isStatic {
			return stripStatic, true
		} else if strings.HasSuffix(binary, ".ko") || strings.HasSuffix(binary, ".o") {
			return stripShared, false
		}
	}

	return "", false
}
