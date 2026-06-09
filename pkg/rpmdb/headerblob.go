package rpmdb

import (
	"encoding/binary"
	"errors"
)

// RPM header tag/type constants — the minimal subset needed to answer
// installed-package queries from a BerkeleyDB header image.
const (
	tagName        = 1000
	tagProvideName = 1047

	typeString      = 6
	typeStringArray = 8
	typeI18nString  = 9

	// entrySize is the on-disk size of one header index entry:
	// tag(4) + type(4) + offset(4) + count(4), all big-endian.
	entrySize = 16

	// maxIndexEntries and maxDataSize bound a header image so a corrupt
	// blob cannot trigger a huge allocation. rpm itself caps the index at
	// 64k entries and the data region at 256 MB.
	maxIndexEntries = 0xFFFF
	maxDataSize     = 256 << 20
)

// errMalformedHeader is returned for blobs that do not parse as an RPM
// header image.
var errMalformedHeader = errors.New("rpmdb: malformed header image")

// headerInfo holds the fields extracted from one rpmdb header image.
type headerInfo struct {
	Name     string
	Provides []string
}

// parseHeaderBlob extracts NAME and PROVIDENAME from an rpmdb header image.
//
// Layout (all integers big-endian; unlike file headers there is no magic
// preamble): il(4) dl(4), then il index entries of 16 bytes each
// (tag, type, offset, count), then a data region of dl bytes. Entry offsets
// are relative to the start of the data region. Region tags (tag < 100,
// e.g. HEADER_IMMUTABLE) are skipped — plain tag reads don't need region
// semantics.
func parseHeaderBlob(blob []byte) (headerInfo, error) {
	var info headerInfo

	if len(blob) < 8 {
		return info, errMalformedHeader
	}

	il := binary.BigEndian.Uint32(blob[0:4])
	dl := binary.BigEndian.Uint32(blob[4:8])

	if il == 0 || il > maxIndexEntries || dl > maxDataSize {
		return info, errMalformedHeader
	}

	indexEnd := 8 + int(il)*entrySize
	if len(blob) < indexEnd || len(blob)-indexEnd < int(dl) {
		return info, errMalformedHeader
	}

	data := blob[indexEnd : indexEnd+int(dl)]

	for i := range int(il) {
		entry := blob[8+i*entrySize : 8+(i+1)*entrySize]
		if err := applyHeaderEntry(&info, data, entry); err != nil {
			return info, err
		}
	}

	if info.Name == "" {
		return info, errMalformedHeader
	}

	return info, nil
}

// applyHeaderEntry decodes one 16-byte index entry and, when it carries a
// tag of interest (NAME, PROVIDENAME), reads its value from the data
// region into info. Other tags — including region/bookkeeping entries —
// are ignored.
func applyHeaderEntry(info *headerInfo, data, entry []byte) error {
	// Tag and offset are signed per the RPM header spec; the uint32→int32
	// conversions reinterpret the big-endian bytes.
	tag := int32(binary.BigEndian.Uint32(entry[0:4])) //nolint:gosec
	typ := binary.BigEndian.Uint32(entry[4:8])
	offset := int32(binary.BigEndian.Uint32(entry[8:12])) //nolint:gosec
	count := binary.BigEndian.Uint32(entry[12:16])

	switch tag {
	case tagName:
		if typ != typeString && typ != typeI18nString {
			return nil
		}

		s, err := readStrings(data, offset, 1)
		if err != nil {
			return err
		}

		info.Name = s[0]

	case tagProvideName:
		if typ != typeStringArray {
			return nil
		}

		s, err := readStrings(data, offset, count)
		if err != nil {
			return err
		}

		info.Provides = s
	}

	return nil
}

// readStrings reads count consecutive NUL-terminated strings from data
// starting at offset.
func readStrings(data []byte, offset int32, count uint32) ([]string, error) {
	if offset < 0 || int(offset) >= len(data) {
		return nil, errMalformedHeader
	}

	out := make([]string, 0, count)
	pos := int(offset)

	for range count {
		end := pos

		for end < len(data) && data[end] != 0 {
			end++
		}

		if end == len(data) {
			return nil, errMalformedHeader
		}

		out = append(out, string(data[pos:end]))
		pos = end + 1
	}

	return out, nil
}
