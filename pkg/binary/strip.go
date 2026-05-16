// Package binary provides binary file manipulation utilities.
package binary //nolint:revive // intentional name; conflicts with stdlib encoding/binary but scope is unambiguous

import (
	"context"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// hostElfMachine returns the ELF machine constant for the current host architecture.
// It returns elf.EM_NONE for architectures we don't have a mapping for, which
// disables the foreign-arch detection (we fall back to attempting strip).
func hostElfMachine() elf.Machine {
	switch runtime.GOARCH {
	case "amd64":
		return elf.EM_X86_64
	case "386":
		return elf.EM_386
	case "arm64":
		return elf.EM_AARCH64
	case "arm":
		return elf.EM_ARM
	case "ppc64le", "ppc64":
		return elf.EM_PPC64
	case "s390x":
		return elf.EM_S390
	case "riscv64":
		return elf.EM_RISCV
	default:
		return elf.EM_NONE
	}
}

// isForeignArchELF returns true if the file is an ELF binary whose architecture
// differs from the build host. Returns false for non-ELF files, unreadable files,
// or when the host architecture isn't mapped — the strip pass then proceeds
// normally (and fails loudly if there's a genuine problem).
func isForeignArchELF(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false // not an ELF or unreadable; let strip handle it
	}

	defer func() { _ = f.Close() }()

	host := hostElfMachine()
	if host == elf.EM_NONE {
		return false
	}

	return f.Machine != host
}

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
	// Use cross-compilation strip if STRIP environment variable is set.
	// This allows stripping binaries compiled for foreign architectures.
	stripCmd := os.Getenv("STRIP")
	if stripCmd == "" {
		stripCmd = "strip"
	} else if _, err := exec.LookPath(stripCmd); err != nil {
		// Cross-strip not installed. Two cases:
		//   (a) The binary is host-arch (pre-built package that doesn't actually
		//       need a cross toolchain): native strip works correctly.
		//   (b) The binary is foreign-arch (real cross-compile output): native
		//       strip cannot parse it and will hard-fail. Skip with a warning
		//       rather than break the build — strip is an optional optimization,
		//       not a correctness requirement.
		if isForeignArchELF(path) {
			logger.Warn(
				"cross-strip not found and binary is foreign-arch; skipping strip",
				"cross_strip", stripCmd,
				"binary", path)

			return nil
		}

		logger.Warn("cross-strip not found in PATH, falling back to native strip",
			"cross_strip", stripCmd)

		stripCmd = "strip"
	}

	args = append(args, path)

	return shell.Exec(context.Background(), false, "", stripCmd, args...)
}
