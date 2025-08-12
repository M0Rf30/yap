// Package options provides build and packaging option utilities,
// including binary stripping functionality.
package options

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	binutil "github.com/M0Rf30/yap/v2/pkg/binary"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Strip walks through the directory to process each file.
func Strip(packageDir string) error {
	logger.Info("stripping binaries")

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
	logger.Debug(
		"ensuring file is writable before stripping",
		"file", binary)

	info, err := os.Stat(binary)
	if err != nil {
		logger.Warn(
			"failed to get file info",
			"file", binary, "error", err)

		return nil
	}

	chmodErr := files.Chmod(binary, info.Mode().Perm()|0o200)
	if chmodErr != nil {
		logger.Warn(
			"failed to make file writable",
			"file", binary, "error", chmodErr)

		return nil // Skip if we can't change permissions
	}

	err = files.CheckWritable(binary)
	if err != nil {
		logger.Warn(
			"file still not writable after chmod",
			"file", binary, "error", err)

		return nil // Skip if not writable
	}

	fileType := files.GetFileType(binary)
	if fileType == "" || fileType == "ET_NONE" {
		return err
	}

	stripFlags, stripLTO := determineStripFlags(fileType, binary)

	logger.Debug(
		"about to strip binary",
		"file", binary, "flags", stripFlags)

	err = binutil.StripFile(binary, stripFlags)
	if err != nil {
		logger.Error(
			"strip command failed",
			"file", binary, "flags", stripFlags, "error", err)

		return err
	}

	if stripLTO {
		err := binutil.StripLTO(binary)
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
		isStatic := files.IsStaticLibrary(binary)
		if isStatic {
			return stripStatic, true
		} else if strings.HasSuffix(binary, ".ko") || strings.HasSuffix(binary, ".o") {
			return stripShared, false
		}
	}

	return "", false
}
