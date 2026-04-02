package options

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/osutils"
)

// debugDir holds the output directory for separated debug symbols.
// When empty, debug info separation is skipped and binaries are stripped normally.
var debugDir string

// SetDebugDir sets the output directory for debug symbols.
func SetDebugDir(dir string) {
	debugDir = dir
}

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

	err = osutils.CheckWritable(binary)
	if err != nil {
		//nolint:nilerr
		return nil // Skip if not writable
	}

	fileType := osutils.GetFileType(binary)
	if fileType == "" || fileType == "ET_NONE" {
		return err
	}

	// Separate debug info before stripping, if a debug directory is configured.
	if debugDir != "" {
		debugFile, sepErr := osutils.SeparateDebugInfo(binary, debugDir)
		if sepErr != nil {
			osutils.Logger.Warn("failed to separate debug info",
				osutils.Logger.Args("binary", binary, "error", sepErr))
		} else if debugFile != "" {
			osutils.Logger.Info("separated debug info",
				osutils.Logger.Args("binary", binary, "debug", debugFile))
		}
	}

	stripFlags, stripLTO := determineStripFlags(fileType, binary)

	err = osutils.StripFile(binary, stripFlags)
	if err != nil {
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
