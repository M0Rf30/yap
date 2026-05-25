// Package options provides build and packaging option utilities,
// including binary stripping functionality.
package options

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	binutil "github.com/M0Rf30/yap/v2/pkg/binary"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// debugDir holds the output directory for separated debug symbols.
var (
	debugDir   string
	debugDirMu sync.RWMutex
)

// SetDebugDir sets the output directory for debug symbols.
// This function is safe to call concurrently from multiple goroutines.
func SetDebugDir(dir string) {
	debugDirMu.Lock()
	defer debugDirMu.Unlock()

	debugDir = dir
}

// Strip walks through the directory to process each file. STRIP/OBJCOPY are
// read from the process environment.
func Strip(packageDir string) error {
	return StripWithEnv(packageDir, nil)
}

// StripWithEnv is the env-overlay variant of Strip. STRIP/OBJCOPY are looked up
// in env first, falling back to os.Getenv when absent. Makes parallel builds
// safe: each goroutine can pass its own toolchain env without process-env
// mutation.
func StripWithEnv(packageDir string, env map[string]string) error {
	logger.Info(i18n.T("logger.strip.info.stripping_binaries_1"))

	return filepath.WalkDir(packageDir, func(p string, d fs.DirEntry, err error) error {
		return processFileWithEnv(p, d, err, env)
	})
}

// processFile is the legacy entry point that reads STRIP/OBJCOPY from os.Getenv.
func processFile(binary string, dirEntry fs.DirEntry, err error) error {
	return processFileWithEnv(binary, dirEntry, err, nil)
}

// processFileWithEnv processes a single file with an optional env overlay for
// STRIP/OBJCOPY. env=nil falls back to os.Getenv (legacy behavior).
func processFileWithEnv(binary string, dirEntry fs.DirEntry, err error, env map[string]string) error {
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

	// Separate debug info before stripping, if a debug directory is configured.
	// Read debugDir under lock to prevent races with SetDebugDir in parallel builds.
	debugDirMu.RLock()

	localDebugDir := debugDir

	debugDirMu.RUnlock()

	if localDebugDir != "" {
		debugFile, sepErr := binutil.SeparateDebugInfoWithEnv(binary, localDebugDir, env)
		if sepErr != nil {
			logger.Warn("failed to separate debug info",
				"binary", binary, "error", sepErr)
		} else if debugFile != "" {
			logger.Info("separated debug info",
				"binary", binary, "debug", debugFile)
		}
	}

	stripFlags, stripLTO := determineStripFlags(fileType, binary)

	logger.Debug(
		"about to strip binary",
		"file", binary, "flags", stripFlags)

	err = binutil.StripFileWithEnv(binary, env, stripFlags)
	if err != nil {
		logger.Error(
			"strip command failed",
			"file", binary, "flags", stripFlags, "error", err)

		return err
	}

	if stripLTO {
		err := binutil.StripLTOWithEnv(binary, env)
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
	const (
		stripBinaries = "--strip-all"
		stripShared   = "--strip-unneeded"
		stripStatic   = "--strip-debug"
	)

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
