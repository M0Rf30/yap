// Package binary provides binary file manipulation utilities.
package binary //nolint:revive // intentional name; conflicts with stdlib encoding/binary but scope is unambiguous

import (
	"context"
	"debug/elf"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// ReadBuildID reads the ELF build-id from the given binary.
// Returns an empty string if the binary has no build-id.
func ReadBuildID(path string) string {
	file, err := elf.Open(path)
	if err != nil {
		return ""
	}

	defer func() {
		_ = file.Close()
	}()

	for _, section := range file.Sections {
		if section.Name != ".note.gnu.build-id" {
			continue
		}

		data, err := section.Data()
		if err != nil || len(data) < 16 {
			return ""
		}

		// ELF note format: namesz(4) + descsz(4) + type(4) + name + desc
		nameSize := file.ByteOrder.Uint32(data[0:4])
		descSize := file.ByteOrder.Uint32(data[4:8])

		nameEnd := 12 + nameSize
		if nameEnd%4 != 0 {
			nameEnd += 4 - nameEnd%4
		}

		descEnd := nameEnd + descSize
		if int(descEnd) > len(data) {
			return ""
		}

		desc := data[nameEnd:descEnd]
		hexStr := make([]byte, len(desc)*2)

		const hexChars = "0123456789abcdef"

		for i, b := range desc {
			hexStr[i*2] = hexChars[b>>4]
			hexStr[i*2+1] = hexChars[b&0x0f]
		}

		return string(hexStr)
	}

	return ""
}

// SeparateDebugInfo extracts debug information from the binary into a separate
// file organized by build-id, then adds a .gnu_debuglink to the original binary.
// The debug file is stored at <debugDir>/.build-id/<prefix>/<suffix>.debug.
// Returns the path to the debug file, or empty string if no build-id was found.
func SeparateDebugInfo(binary, debugDir string) (string, error) {
	buildID := ReadBuildID(binary)
	if buildID == "" || len(buildID) < 3 {
		return "", nil
	}

	prefix := buildID[:2]
	suffix := buildID[2:]
	debugSubDir := filepath.Join(debugDir, ".build-id", prefix)

	err := files.ExistsMakeDir(debugSubDir)
	if err != nil {
		return "", err
	}

	debugFile := filepath.Join(debugSubDir, suffix+".debug")

	// Use cross-compilation objcopy if OBJCOPY environment variable is set
	objcopyCmd := os.Getenv("OBJCOPY")
	if objcopyCmd == "" {
		objcopyCmd = "objcopy"
	}

	ctx := context.Background()

	err = shell.Exec(ctx, false, "", objcopyCmd, "--only-keep-debug", binary, debugFile)
	if err != nil {
		return "", err
	}

	err = shell.Exec(ctx, false, "", objcopyCmd, "--add-gnu-debuglink="+debugFile, binary)
	if err != nil {
		logger.Warn("failed to add debuglink",
			"binary", binary, "error", err)
	}

	return debugFile, nil
}

// StripFile removes debugging symbols from a binary file.
// It respects the STRIP environment variable for cross-compilation support.
func StripFile(path string, args ...string) error {
	return strip(path, args...)
}

// StripLTO removes LTO (Link Time Optimization) sections from a binary file.
// It respects the STRIP environment variable for cross-compilation support.
func StripLTO(path string, args ...string) error {
	return strip(
		path,
		append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

func strip(path string, args ...string) error {
	args = append(args, path)

	// Use cross-compilation strip if STRIP environment variable is set
	// This allows stripping binaries compiled for foreign architectures
	stripCmd := os.Getenv("STRIP")
	if stripCmd == "" {
		stripCmd = "strip"
	}

	return shell.Exec(context.Background(), false, "", stripCmd, args...)
}
