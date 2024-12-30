package options

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/utils"
)

// Strip walks through the directory to process each file.
func Strip(packageDir string) error {
	utils.Logger.Info("stripping binaries")

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

	if err := utils.CheckWritable(binary); err != nil {
		//nolint:nilerr
		return nil // Skip if not writable
	}

	fileType := utils.GetFileType(binary)
	if fileType == "" || fileType == "ET_NONE" {
		return err
	}

	stripFlags, stripLTO := determineStripFlags(fileType, binary)

	if err := utils.StripFile(binary, stripFlags); err != nil {
		return err
	}

	if stripLTO {
		if err := utils.StripLTO(binary); err != nil {
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
		isStatic := utils.IsStaticLibrary(binary)
		if isStatic {
			return stripStatic, true
		} else if strings.HasSuffix(binary, ".ko") || strings.HasSuffix(binary, ".o") {
			return stripShared, false
		}
	}

	return "", false
}
