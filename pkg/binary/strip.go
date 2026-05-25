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

// lookupEnv returns env[key] if present, otherwise os.Getenv(key).
// Allows callers to override STRIP/OBJCOPY without mutating the process env.
func lookupEnv(env map[string]string, key string) string {
	if env != nil {
		if v, ok := env[key]; ok {
			return v
		}
	}

	return os.Getenv(key)
}

// SeparateDebugInfo extracts debug information from the binary using OBJCOPY
// from os.Getenv. See SeparateDebugInfoWithEnv for the env-overlay variant.
func SeparateDebugInfo(binary, debugDir string) (string, error) {
	return SeparateDebugInfoWithEnv(binary, debugDir, nil)
}

// SeparateDebugInfoWithEnv extracts debug information from the binary into a separate
// file organized by build-id, then adds a .gnu_debuglink to the original binary.
// The debug file is stored at <debugDir>/.build-id/<prefix>/<suffix>.debug.
// Returns the path to the debug file, or empty string if no build-id was found.
// OBJCOPY is read from env (with os.Getenv fallback) to support parallel builds
// without process-env mutation.
func SeparateDebugInfoWithEnv(binary, debugDir string, env map[string]string) (string, error) {
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

	// Use cross-compilation objcopy if OBJCOPY is set (env-map overlay or os.Getenv).
	objcopyCmd := lookupEnv(env, "OBJCOPY")
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
	return stripWithEnv(path, nil, args...)
}

// StripFileWithEnv is the env-overlay variant of StripFile. STRIP is read from
// env first (parallel-build safe), with os.Getenv as fallback.
func StripFileWithEnv(path string, env map[string]string, args ...string) error {
	return stripWithEnv(path, env, args...)
}

// StripLTO removes LTO (Link Time Optimization) sections from a binary file.
// It respects the STRIP environment variable for cross-compilation support.
func StripLTO(path string, args ...string) error {
	return stripWithEnv(
		path, nil,
		append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

// StripLTOWithEnv is the env-overlay variant of StripLTO.
func StripLTOWithEnv(path string, env map[string]string, args ...string) error {
	return stripWithEnv(
		path, env,
		append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

func stripWithEnv(path string, env map[string]string, args ...string) error {
	// Use cross-compilation strip if STRIP is configured (env-map overlay or os.Getenv).
	stripCmd := lookupEnv(env, "STRIP")

	switch {
	case stripCmd == "":
		// No cross-strip configured. Check whether the binary is foreign-arch
		// before invoking the native strip — native strip cannot parse ELF
		// binaries built for a different architecture and will hard-fail.
		// Strip is an optional optimization, not a correctness requirement,
		// so we skip with a warning rather than break the build.
		if isForeignArchELF(path) {
			logger.Warn(
				"skipping strip: binary is foreign-arch and no cross-strip configured",
				"binary", path)

			return nil
		}

		stripCmd = "strip"

	case stripCmd != "":
		if _, err := exec.LookPath(stripCmd); err != nil {
			// Cross-strip configured but not installed. Same two cases as above.
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
	}

	args = append(args, path)

	return shell.Exec(context.Background(), false, "", stripCmd, args...)
}
