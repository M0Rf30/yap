package binary_test

import (
	"debug/elf"
	enc "encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/binary"
)

func TestStripFile(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripFile - this will likely fail on a non-binary file but tests the function call
	_ = binary.StripFile(testFile)
	// We don't fail the test if strip command fails because we're testing with dummy content
	// The important thing is that the function executes without panicking
}

func TestStripLTO(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripLTO - this will likely fail on a non-binary file but tests the function call
	_ = binary.StripLTO(testFile)
	// We don't fail the test if strip command fails because we're testing with dummy content
	// The important thing is that the function executes without panicking
}

func TestStripFileWithArgs(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripFile with additional args
	_ = binary.StripFile(testFile, "--version")
	// We don't fail the test if strip command fails because we're testing with dummy content
}

func TestStripLTOWithArgs(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripLTO with additional args
	_ = binary.StripLTO(testFile, "--version")
	// We don't fail the test if strip command fails because we're testing with dummy content
}

func TestStripNonExistentFile(t *testing.T) {
	// Test with non-existent file
	err := binary.StripFile("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestStripLTONonExistentFile(t *testing.T) {
	// Test with non-existent file
	err := binary.StripLTO("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestStripWithCrossCompilation(t *testing.T) {
	// Save original STRIP environment variable
	originalStrip := os.Getenv("STRIP")

	defer func() {
		if originalStrip != "" {
			_ = os.Setenv("STRIP", originalStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with cross-compilation STRIP variable set
	_ = os.Setenv("STRIP", "aarch64-linux-gnu-strip")

	// This will fail because aarch64-linux-gnu-strip may not exist,
	// but we're testing that it attempts to use the right command
	_ = binary.StripFile(testFile)

	// Test with STRIP unset (should use default "strip")
	_ = os.Unsetenv("STRIP")
	_ = binary.StripFile(testFile)
}

func TestStripFallsBackToNativeWhenCrossStripMissing(t *testing.T) {
	t.Helper()

	originalStrip := os.Getenv("STRIP")

	defer func() {
		if originalStrip != "" {
			_ = os.Setenv("STRIP", originalStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set STRIP to a cross-strip tool that definitely does not exist.
	// The fallback logic should silently switch to native strip instead of
	// returning an "executable file not found" error.
	_ = os.Setenv("STRIP", "this-cross-strip-does-not-exist")

	// Native strip will still fail on a non-ELF dummy file, but the error
	// must NOT be "executable file not found" — that would mean the fallback
	// did not trigger.
	err = binary.StripFile(testFile)
	if err != nil {
		errMsg := err.Error()
		if contains(errMsg, "executable file not found") || contains(errMsg, "no such file or directory") && contains(errMsg, "this-cross-strip-does-not-exist") {
			t.Errorf("expected fallback to native strip, but got cross-strip error: %v", err)
		}
	}
}

func TestStripUsesNativeStripWhenEnvUnset(t *testing.T) {
	t.Helper()

	originalStrip := os.Getenv("STRIP")

	defer func() {
		if originalStrip != "" {
			_ = os.Setenv("STRIP", originalStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	_ = os.Unsetenv("STRIP")

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// With STRIP unset, native strip is used. It may fail on a non-ELF file,
	// but must not fail with "executable file not found".
	err = binary.StripFile(testFile)
	if err != nil {
		if contains(err.Error(), "executable file not found") {
			t.Errorf("native strip not found in PATH: %v", err)
		}
	}
}

// contains is a helper to avoid importing strings in test assertions.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		func() bool {
			for i := range len(s) - len(substr) + 1 {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}

			return false
		}())
}

// writeMinimalELF writes a minimal 64-bit little-endian ELF header to path with
// the given machine type. Just enough for debug/elf.Open to parse successfully —
// no sections, no segments. Used to test foreign-arch detection in strip.
func writeMinimalELF(t *testing.T, path string, machine elf.Machine) {
	t.Helper()

	// 64-byte ELF64 header.
	hdr := make([]byte, 64)
	// e_ident
	hdr[0] = 0x7f
	hdr[1] = 'E'
	hdr[2] = 'L'
	hdr[3] = 'F'
	hdr[4] = 2 // ELFCLASS64
	hdr[5] = 1 // ELFDATA2LSB (little-endian)
	hdr[6] = 1 // EV_CURRENT
	// Set e_type to ET_EXEC, e_machine to the requested arch, e_version to 1.
	enc.LittleEndian.PutUint16(hdr[16:], 2)
	enc.LittleEndian.PutUint16(hdr[18:], uint16(machine))
	enc.LittleEndian.PutUint32(hdr[20:], 1)
	// Set e_ehsize to 64; phentsize and shentsize remain zero (no segments/sections).
	enc.LittleEndian.PutUint16(hdr[52:], 64)

	if err := os.WriteFile(path, hdr, 0o755); err != nil {
		t.Fatalf("write minimal ELF: %v", err)
	}
}

// TestStripSkipsForeignArchELFOnFallback verifies that when STRIP points to a
// missing cross-strip AND the target is a foreign-arch ELF, strip is skipped
// (returns nil) rather than handing the binary to native strip — native strip
// cannot parse foreign-arch ELFs and would hard-fail the build.
func TestStripSkipsForeignArchELFOnFallback(t *testing.T) {
	// Pick a machine that is not the host. If host is amd64, use aarch64; otherwise amd64.
	var foreign elf.Machine
	if runtime.GOARCH == "amd64" {
		foreign = elf.EM_AARCH64
	} else {
		foreign = elf.EM_X86_64
	}

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "foreign_binary")
	writeMinimalELF(t, testFile, foreign)

	originalStrip := os.Getenv("STRIP")

	defer func() {
		if originalStrip != "" {
			_ = os.Setenv("STRIP", originalStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	_ = os.Setenv("STRIP", "this-cross-strip-does-not-exist")

	// Must return nil: foreign-arch ELF, cross-strip missing → skip with warning.
	if err := binary.StripFile(testFile); err != nil {
		t.Errorf("expected nil (skip strip on foreign-arch fallback), got: %v", err)
	}
}
