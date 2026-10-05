// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package rpmdb //nolint:testpackage

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/M0Rf30/rpmpack"
	rpmutils "github.com/sassoftware/go-rpmutils"
)

// buildTestRPM builds a real RPM in memory with provides and returns it
// parsed by go-rpmutils.
func buildTestRPM(t *testing.T) *rpmutils.Rpm {
	t.Helper()

	provides := rpmpack.Relations{}
	if err := provides.Set("test-pkg-lib"); err != nil {
		t.Fatal(err)
	}

	pkg, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:       "test-pkg",
		Version:    "1.2.3",
		Release:    "4",
		Arch:       "x86_64",
		Summary:    "Round-trip test package",
		Compressor: "gzip",
		BuildTime:  time.Unix(1700000000, 0),
		Provides:   provides,
	})
	if err != nil {
		t.Fatal(err)
	}

	pkg.AddFile(rpmpack.RPMFile{
		Name: "/usr/bin/test", Body: []byte("#!/bin/sh\n"), Mode: 0o755,
	})

	var buf bytes.Buffer
	if err := pkg.Write(&buf); err != nil {
		t.Fatal(err)
	}

	rpm, err := rpmutils.ReadRpm(&buf)
	if err != nil {
		t.Fatal(err)
	}

	return rpm
}

type parsedEntry struct {
	typ    uint32
	offset int32
	count  uint32
}

// indexBlob decodes the index of an rpmdb header image, asserting the
// structural invariants rpm enforces (sorted tags, aligned, in-range
// offsets) and returns the entries and the data region.
func indexBlob(t *testing.T, blob []byte) (entries map[int32]parsedEntry, data []byte) {
	t.Helper()

	il := int(binary.BigEndian.Uint32(blob[0:4]))
	dl := int(binary.BigEndian.Uint32(blob[4:8]))
	end := 8 + il*entrySize

	if len(blob) != end+dl {
		t.Fatalf("blob length %d != 8+%d*16+%d", len(blob), il, dl)
	}

	data = blob[end:]
	got := make(map[int32]parsedEntry, il)
	prev := int32(-1)
	align := map[uint32]int32{3: 2, 4: 4, 5: 8}

	for i := range il {
		e := blob[8+i*entrySize:]
		tag := int32(binary.BigEndian.Uint32(e[0:4]))
		pe := parsedEntry{
			typ:    binary.BigEndian.Uint32(e[4:8]),
			offset: int32(binary.BigEndian.Uint32(e[8:12])),
			count:  binary.BigEndian.Uint32(e[12:16]),
		}

		if tag <= prev {
			t.Fatalf("tags not strictly ascending: %d after %d", tag, prev)
		}

		prev = tag

		if pe.offset < 0 || int(pe.offset) >= len(data) {
			t.Fatalf("tag %d offset %d out of range", tag, pe.offset)
		}

		if a := align[pe.typ]; a > 0 && pe.offset%a != 0 {
			t.Fatalf("tag %d type %d offset %d misaligned", tag, pe.typ, pe.offset)
		}

		got[tag] = pe
	}

	return got, data
}

// TestSerializeHeaderRoundTrip verifies the blob has no file-header
// preamble and is accepted by parseHeaderBlob (the rpmdb Packages format).
func TestSerializeHeaderRoundTrip(t *testing.T) {
	rpm := buildTestRPM(t)

	files := []InstalledFile{
		{Path: "/usr/bin/test", Size: 1024, Mode: 0o755, User: "root", Group: "root",
			SHA256: "abc123", MTime: time.Now()},
		{Path: "/usr/share/test", Mode: uint32(os.ModeDir | 0o755)},
		{Path: "/usr/bin/link", LinkTarget: "test", Mode: uint32(os.ModeSymlink | 0o777)},
	}

	blob, err := serializeHeader(rpm, files)
	if err != nil {
		t.Fatalf("serializeHeader: %v", err)
	}

	if bytes.HasPrefix(blob, []byte{0x8e, 0xad, 0xe8, 0x01}) {
		t.Fatal("rpmdb header blob must not carry the file-header magic")
	}

	info, err := parseHeaderBlob(blob)
	if err != nil {
		t.Fatalf("parseHeaderBlob rejected serialized header: %v", err)
	}

	if info.Name != "test-pkg" {
		t.Errorf("Name = %q, want test-pkg", info.Name)
	}

	if len(info.Provides) == 0 || info.Provides[0] != "test-pkg-lib" {
		t.Errorf("Provides = %v, want [test-pkg-lib ...]", info.Provides)
	}

	entries, data := indexBlob(t, blob)

	wantTypes := map[int32]uint32{
		rpmutils.SIZE:           4,
		rpmutils.BUILDTIME:      4,
		rpmutils.FILEMODES:      3,
		rpmutils.FILESIZES:      4,
		rpmutils.FILEMTIMES:     4,
		rpmutils.DIRINDEXES:     4,
		rpmutils.FILEDIGESTALGO: 4,
	}
	for tag, typ := range wantTypes {
		e, ok := entries[tag]
		if !ok {
			t.Errorf("tag %d missing", tag)

			continue
		}

		if e.typ != typ {
			t.Errorf("tag %d type = %d, want %d", tag, e.typ, typ)
		}
	}

	for _, tag := range []int32{rpmutils.LONGSIZE, rpmutils.LONGFILESIZES} {
		if _, ok := entries[tag]; ok {
			t.Errorf("unexpected INT64 tag %d for small sizes", tag)
		}
	}

	fm := entries[rpmutils.FILEMODES]
	modes := make([]uint16, fm.count)

	for i := range modes {
		modes[i] = binary.BigEndian.Uint16(data[int(fm.offset)+2*i:])
	}

	want := []uint16{0o100755, 0o040755, 0o120777}
	for i := range want {
		if modes[i] != want[i] {
			t.Errorf("FILEMODES[%d] = %#o, want %#o", i, modes[i], want[i])
		}
	}
}

// TestSerializeHeaderLargeFiles verifies INT64 long tags for >2GiB sizes.
func TestSerializeHeaderLargeFiles(t *testing.T) {
	rpm := buildTestRPM(t)

	blob, err := serializeHeader(rpm, []InstalledFile{
		{Path: "/big", Size: 3 << 30, Mode: 0o644},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := indexBlob(t, blob)

	if e, ok := entries[rpmutils.LONGFILESIZES]; !ok || e.typ != 5 {
		t.Errorf("LONGFILESIZES entry = %+v ok=%v, want INT64", e, ok)
	}

	if _, ok := entries[rpmutils.FILESIZES]; ok {
		t.Error("FILESIZES must be absent when LONGFILESIZES is used")
	}
}

// TestPosixFileMode covers the S_IFMT and special-bit mapping.
func TestPosixFileMode(t *testing.T) {
	tests := []struct {
		name string
		mode uint32
		link string
		want uint16
	}{
		{"regular posix", 0o100644, "", 0o100644},
		{"perm only", 0o644, "", 0o100644},
		{"go dir", uint32(os.ModeDir | 0o755), "", 0o040755},
		{"go symlink", uint32(os.ModeSymlink | 0o777), "t", 0o120777},
		{"link target only", 0o777, "t", 0o120777},
		{"go setuid", uint32(os.ModeSetuid | 0o755), "", 0o104755},
		{"go sticky dir", uint32(os.ModeDir | os.ModeSticky | 0o777), "", 0o041777},
		{"raw setgid", 0o2755, "", 0o102755},
	}

	for _, tt := range tests {
		if got := posixFileMode(tt.mode, tt.link); got != tt.want {
			t.Errorf("%s: posixFileMode = %#o, want %#o", tt.name, got, tt.want)
		}
	}
}
