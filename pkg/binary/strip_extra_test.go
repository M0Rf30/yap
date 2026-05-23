package binary_test

import (
	"debug/elf"
	enc "encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/binary"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeELFWithBuildID creates a minimal 64-bit LE ELF with a .note.gnu.build-id
// section containing the given raw build-id bytes.
func writeELFWithBuildID(t *testing.T, path string, buildID []byte) {
	t.Helper()

	// We'll build the file in memory:
	//   ELF64 header (64 bytes)
	//   .note.gnu.build-id section data
	//   section header string table (.shstrtab)
	//   section header table (3 entries: null, .note.gnu.build-id, .shstrtab)

	// --- note section payload ---
	// ELF note: namesz(4) + descsz(4) + type(4) + name("GNU\0") + desc(buildID)
	const noteName = "GNU\x00"

	nameSize := uint32(len(noteName))
	descSize := uint32(len(buildID))
	noteData := make([]byte, 0, 12+len(noteName)+len(buildID))
	tmp4 := make([]byte, 4)
	enc.LittleEndian.PutUint32(tmp4, nameSize)
	noteData = append(noteData, tmp4...)
	enc.LittleEndian.PutUint32(tmp4, descSize)
	noteData = append(noteData, tmp4...)
	enc.LittleEndian.PutUint32(tmp4, 3) // 3 is NT_GNU_BUILD_ID
	noteData = append(noteData, tmp4...)
	noteData = append(noteData, []byte(noteName)...)
	noteData = append(noteData, buildID...)

	// --- shstrtab ---
	// index 0: \0
	// index 1: .note.gnu.build-id\0  (len=19, ends at index 20)
	// index 20: .shstrtab\0
	shstrtab := "\x00.note.gnu.build-id\x00.shstrtab\x00"
	noteNameIdx := uint32(1)
	shstrtabNameIdx := uint32(20)

	// Layout:
	//   0x00 – 0x3f : ELF header (64 bytes)
	//   0x40        : note section data
	//   0x40+len(noteData) : shstrtab
	//   align to 8  : section header table (3 × 64 bytes)

	noteOffset := uint64(64)
	shstrtabOffset := noteOffset + uint64(len(noteData))
	// align shoff to 8
	shoff := shstrtabOffset + uint64(len(shstrtab))
	if shoff%8 != 0 {
		shoff += 8 - shoff%8
	}

	totalSize := shoff + 3*64

	buf := make([]byte, totalSize)

	// ELF header
	buf[0] = 0x7f
	buf[1] = 'E'
	buf[2] = 'L'
	buf[3] = 'F'
	buf[4] = 2                                                  // ELFCLASS64
	buf[5] = 1                                                  // ELFDATA2LSB
	buf[6] = 1                                                  // EV_CURRENT
	enc.LittleEndian.PutUint16(buf[16:], 2)                     // 2 is ET_EXEC
	enc.LittleEndian.PutUint16(buf[18:], uint16(elf.EM_X86_64)) // e_machine
	enc.LittleEndian.PutUint32(buf[20:], 1)                     // e_version
	enc.LittleEndian.PutUint16(buf[52:], 64)                    // e_ehsize
	enc.LittleEndian.PutUint16(buf[58:], 64)                    // e_shentsize
	enc.LittleEndian.PutUint16(buf[60:], 3)                     // e_shnum
	enc.LittleEndian.PutUint16(buf[62:], 2)                     // e_shstrndx
	enc.LittleEndian.PutUint64(buf[40:], shoff)                 // e_shoff

	// Copy note data
	copy(buf[noteOffset:], noteData)
	// Copy shstrtab
	copy(buf[shstrtabOffset:], shstrtab)

	// Section header 0: null (already zeroed)

	// Section header 1: .note.gnu.build-id
	sh1 := buf[shoff+64:]
	enc.LittleEndian.PutUint32(sh1[0:], noteNameIdx)            // sh_name
	enc.LittleEndian.PutUint32(sh1[4:], uint32(elf.SHT_NOTE))   // sh_type
	enc.LittleEndian.PutUint64(sh1[24:], noteOffset)            // sh_offset
	enc.LittleEndian.PutUint64(sh1[32:], uint64(len(noteData))) // sh_size

	// Section header 2: .shstrtab
	sh2 := buf[shoff+128:]
	enc.LittleEndian.PutUint32(sh2[0:], shstrtabNameIdx)        // sh_name
	enc.LittleEndian.PutUint32(sh2[4:], uint32(elf.SHT_STRTAB)) // sh_type
	enc.LittleEndian.PutUint64(sh2[24:], shstrtabOffset)        // sh_offset
	enc.LittleEndian.PutUint64(sh2[32:], uint64(len(shstrtab))) // sh_size

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("writeELFWithBuildID: %v", err)
	}
}

// hostMachineForTest mirrors the logic in hostElfMachine() in strip.go.
func hostMachineForTest() elf.Machine {
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
		return elf.EM_X86_64
	}
}

// ---------------------------------------------------------------------------
// ReadBuildID tests
// ---------------------------------------------------------------------------

func TestReadBuildID_NonExistentFile(t *testing.T) {
	result := binary.ReadBuildID("/nonexistent/path/to/binary")
	if result != "" {
		t.Errorf("expected empty string for non-existent file, got %q", result)
	}
}

func TestReadBuildID_NotELF(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "textfile")

	if err := os.WriteFile(f, []byte("this is not an ELF binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := binary.ReadBuildID(f)
	if result != "" {
		t.Errorf("expected empty string for non-ELF file, got %q", result)
	}
}

func TestReadBuildID_ELFWithoutBuildID(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "no_buildid.elf")
	// writeMinimalELF from strip_test.go creates an ELF with no sections
	writeMinimalELF(t, f, elf.EM_X86_64)

	result := binary.ReadBuildID(f)
	if result != "" {
		t.Errorf("expected empty string for ELF without build-id, got %q", result)
	}
}

func TestReadBuildID_ELFWithBuildID(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "with_buildid.elf")

	// 4-byte build-id → hex "deadbeef"
	buildID := []byte{0xde, 0xad, 0xbe, 0xef}
	writeELFWithBuildID(t, f, buildID)

	result := binary.ReadBuildID(f)
	if result != "deadbeef" {
		t.Errorf("expected build-id %q, got %q", "deadbeef", result)
	}
}

func TestReadBuildID_ELFWithLongerBuildID(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "long_buildid.elf")

	// 20-byte SHA1-style build-id
	buildID := []byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		0x11, 0x22, 0x33, 0x44,
	}
	writeELFWithBuildID(t, f, buildID)

	result := binary.ReadBuildID(f)
	expected := "0123456789abcdeffedcba987654321011223344"

	if result != expected {
		t.Errorf("expected build-id %q, got %q", expected, result)
	}
}

func TestReadBuildID_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "empty")

	if err := os.WriteFile(f, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	result := binary.ReadBuildID(f)
	if result != "" {
		t.Errorf("expected empty string for empty file, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// SeparateDebugInfo tests
// ---------------------------------------------------------------------------

func TestSeparateDebugInfo_NonELF(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "notelf")
	debugDir := filepath.Join(tmp, "debug")

	if err := os.WriteFile(bin, []byte("not an elf"), 0o755); err != nil {
		t.Fatal(err)
	}

	// ReadBuildID returns "" for non-ELF → SeparateDebugInfo returns ("", nil)
	path, err := binary.SeparateDebugInfo(bin, debugDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if path != "" {
		t.Errorf("expected empty path for non-ELF, got %q", path)
	}
}

func TestSeparateDebugInfo_ELFWithoutBuildID(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "no_buildid.elf")
	debugDir := filepath.Join(tmp, "debug")

	writeMinimalELF(t, bin, elf.EM_X86_64)

	path, err := binary.SeparateDebugInfo(bin, debugDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if path != "" {
		t.Errorf("expected empty path for ELF without build-id, got %q", path)
	}
}

func TestSeparateDebugInfo_NonExistentBinary(t *testing.T) {
	tmp := t.TempDir()
	debugDir := filepath.Join(tmp, "debug")

	// Non-existent binary → ReadBuildID returns "" → returns ("", nil)
	path, err := binary.SeparateDebugInfo("/nonexistent/binary", debugDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if path != "" {
		t.Errorf("expected empty path for non-existent binary, got %q", path)
	}
}

func TestSeparateDebugInfo_ELFWithBuildID_ObjcopyMissing(t *testing.T) {
	// If objcopy is not available, SeparateDebugInfo should return an error (not panic).
	origObjcopy := os.Getenv("OBJCOPY")

	defer func() {
		if origObjcopy != "" {
			_ = os.Setenv("OBJCOPY", origObjcopy)
		} else {
			_ = os.Unsetenv("OBJCOPY")
		}
	}()

	_ = os.Setenv("OBJCOPY", "this-objcopy-does-not-exist")

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "with_buildid.elf")
	debugDir := filepath.Join(tmp, "debug")

	buildID := []byte{0xca, 0xfe, 0xba, 0xbe, 0x01, 0x02}
	writeELFWithBuildID(t, bin, buildID)

	_, err := binary.SeparateDebugInfo(bin, debugDir)
	if err == nil {
		t.Error("expected error when objcopy is missing, got nil")
	}
}

func TestSeparateDebugInfo_ELFWithBuildID_DebugDirCreated(t *testing.T) {
	// Verify that the debug subdirectory is created even if objcopy fails.
	origObjcopy := os.Getenv("OBJCOPY")

	defer func() {
		if origObjcopy != "" {
			_ = os.Setenv("OBJCOPY", origObjcopy)
		} else {
			_ = os.Unsetenv("OBJCOPY")
		}
	}()

	_ = os.Setenv("OBJCOPY", "this-objcopy-does-not-exist")

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "with_buildid.elf")
	debugDir := filepath.Join(tmp, "debug")

	buildID := []byte{0xca, 0xfe, 0xba, 0xbe, 0x01, 0x02}
	writeELFWithBuildID(t, bin, buildID)

	_, _ = binary.SeparateDebugInfo(bin, debugDir)

	// The .build-id/<prefix> directory should have been created before objcopy runs.
	// build-id hex = "cafebabe0102", prefix = "ca"
	expectedSubDir := filepath.Join(debugDir, ".build-id", "ca")
	if _, err := os.Stat(expectedSubDir); os.IsNotExist(err) {
		t.Errorf("expected debug subdir %q to be created, but it does not exist", expectedSubDir)
	}
}

func TestSeparateDebugInfo_ShortBuildID(t *testing.T) {
	// 1 raw byte → "ab" (2 hex chars, len < 3) → SeparateDebugInfo returns ("", nil)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "short_buildid.elf")
	debugDir := filepath.Join(tmp, "debug")

	buildID := []byte{0xab}
	writeELFWithBuildID(t, bin, buildID)

	path, err := binary.SeparateDebugInfo(bin, debugDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if path != "" {
		t.Errorf("expected empty path for short build-id, got %q", path)
	}
}

// ---------------------------------------------------------------------------
// isForeignArchELF coverage via StripFile
// ---------------------------------------------------------------------------

// TestStripFile_HostArchELF_CrossStripMissing verifies that when STRIP points to
// a missing cross-strip AND the target is a host-arch ELF, the code falls back
// to native strip (not skip). The error (if any) must come from native strip,
// not from the cross-strip binary being absent.
func TestStripFile_HostArchELF_CrossStripMissing(t *testing.T) {
	origStrip := os.Getenv("STRIP")

	defer func() {
		if origStrip != "" {
			_ = os.Setenv("STRIP", origStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	_ = os.Setenv("STRIP", "this-cross-strip-does-not-exist")

	tmp := t.TempDir()
	f := filepath.Join(tmp, "host_arch.elf")
	writeMinimalELF(t, f, hostMachineForTest())

	err := binary.StripFile(f)
	// If err is non-nil it should be from native strip failing on a minimal ELF,
	// NOT from the cross-strip not being found.
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "this-cross-strip-does-not-exist") {
			t.Errorf("cross-strip name leaked into error; fallback did not trigger: %v", err)
		}
	}
}

// TestStripFile_NonELFFile_CrossStripMissing verifies that a non-ELF file with a
// missing cross-strip falls back to native strip (isForeignArchELF returns false
// for non-ELF files).
func TestStripFile_NonELFFile_CrossStripMissing(t *testing.T) {
	origStrip := os.Getenv("STRIP")

	defer func() {
		if origStrip != "" {
			_ = os.Setenv("STRIP", origStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	_ = os.Setenv("STRIP", "this-cross-strip-does-not-exist")

	tmp := t.TempDir()
	f := filepath.Join(tmp, "plaintext")

	if err := os.WriteFile(f, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := binary.StripFile(f)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "this-cross-strip-does-not-exist") {
			t.Errorf("cross-strip name leaked into error for non-ELF file: %v", err)
		}
	}
}
