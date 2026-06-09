//nolint:testpackage
package rpmdb

import (
	"encoding/binary"
	"testing"
)

// blobEntry describes one index entry for buildHeaderBlob.
type blobEntry struct {
	tag    int32
	typ    uint32
	offset int32
	count  uint32
}

// buildHeaderBlob assembles a synthetic rpmdb header image from index
// entries and a raw data region.
func buildHeaderBlob(entries []blobEntry, data []byte) []byte {
	blob := make([]byte, 0, 8+len(entries)*entrySize+len(data))

	blob = binary.BigEndian.AppendUint32(blob, uint32(len(entries)))
	blob = binary.BigEndian.AppendUint32(blob, uint32(len(data)))

	for _, e := range entries {
		blob = binary.BigEndian.AppendUint32(blob, uint32(e.tag))
		blob = binary.BigEndian.AppendUint32(blob, e.typ)
		blob = binary.BigEndian.AppendUint32(blob, uint32(e.offset))
		blob = binary.BigEndian.AppendUint32(blob, e.count)
	}

	return append(blob, data...)
}

// TestParseHeaderBlobNameAndProvides tests extraction of NAME and
// PROVIDENAME from a well-formed header image.
func TestParseHeaderBlobNameAndProvides(t *testing.T) {
	// data region: "tree\0" at 0, provides "tree\0tree(x86-64)\0" at 5.
	data := []byte("tree\x00tree\x00tree(x86-64)\x00")

	blob := buildHeaderBlob([]blobEntry{
		{tag: tagName, typ: typeString, offset: 0, count: 1},
		{tag: tagProvideName, typ: typeStringArray, offset: 5, count: 2},
	}, data)

	info, err := parseHeaderBlob(blob)
	if err != nil {
		t.Fatalf("parseHeaderBlob failed: %v", err)
	}

	if info.Name != "tree" {
		t.Errorf("Name = %q, want %q", info.Name, "tree")
	}

	if len(info.Provides) != 2 || info.Provides[1] != "tree(x86-64)" {
		t.Errorf("unexpected Provides: %v", info.Provides)
	}
}

// TestParseHeaderBlobSkipsRegionTag tests that region/bookkeeping entries
// (tag < 100) are ignored without affecting NAME extraction.
func TestParseHeaderBlobSkipsRegionTag(t *testing.T) {
	data := append(make([]byte, 16), []byte("bash\x00")...) // region trailer + name

	blob := buildHeaderBlob([]blobEntry{
		{tag: 63, typ: 7 /* BIN */, offset: 0, count: 16}, // HEADER_IMMUTABLE region
		{tag: tagName, typ: typeString, offset: 16, count: 1},
	}, data)

	info, err := parseHeaderBlob(blob)
	if err != nil {
		t.Fatalf("parseHeaderBlob failed: %v", err)
	}

	if info.Name != "bash" {
		t.Errorf("Name = %q, want %q", info.Name, "bash")
	}
}

// TestParseHeaderBlobMalformed tests rejection of truncated or corrupt blobs.
func TestParseHeaderBlobMalformed(t *testing.T) {
	cases := []struct {
		name string
		blob []byte
	}{
		{"empty", nil},
		{"too short", []byte{0, 0, 0, 1}},
		{"huge index count", buildHeaderBlob(nil, nil)}, // il==0
		{"truncated index", func() []byte {
			b := buildHeaderBlob([]blobEntry{{tag: tagName, typ: typeString}}, []byte("x\x00"))
			return b[:12]
		}()},
		{"no name tag", buildHeaderBlob([]blobEntry{
			{tag: 1001 /* VERSION */, typ: typeString, offset: 0, count: 1},
		}, []byte("1.0\x00"))},
		{"offset out of range", buildHeaderBlob([]blobEntry{
			{tag: tagName, typ: typeString, offset: 99, count: 1},
		}, []byte("x\x00"))},
		{"unterminated string", buildHeaderBlob([]blobEntry{
			{tag: tagName, typ: typeString, offset: 0, count: 1},
		}, []byte("no-nul"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseHeaderBlob(tc.blob); err == nil {
				t.Error("expected error for malformed blob")
			}
		})
	}
}

// TestParseHeaderBlobNegativeProvidesCountSafe tests that a hostile count
// with an out-of-range offset cannot panic.
func TestParseHeaderBlobNegativeProvidesCountSafe(t *testing.T) {
	blob := buildHeaderBlob([]blobEntry{
		{tag: tagName, typ: typeString, offset: 0, count: 1},
		{tag: tagProvideName, typ: typeStringArray, offset: -5, count: 1 << 30},
	}, []byte("pkg\x00"))

	if _, err := parseHeaderBlob(blob); err == nil {
		t.Error("expected error for negative offset")
	}
}

// TestOpenLegacyMissing tests that OpenLegacy returns ErrNoBDB when no BDB
// database exists (the common case on test machines).
func TestOpenLegacyMissing(t *testing.T) {
	orig := bdbPaths

	bdbPaths = []string{"/nonexistent/rpm/Packages"}

	t.Cleanup(func() { bdbPaths = orig })

	if _, err := OpenLegacy(); err == nil {
		t.Error("expected ErrNoBDB")
	}
}
