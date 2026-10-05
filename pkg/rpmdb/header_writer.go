// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package rpmdb

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"slices"

	rpmutils "github.com/sassoftware/go-rpmutils"
)

// POSIX file type bits (S_IFMT and friends) as stored in RPMTAG_FILEMODES.
const (
	sIFMT   = 0o170000
	sIFSOCK = 0o140000
	sIFLNK  = 0o120000
	sIFREG  = 0o100000
	sIFBLK  = 0o060000
	sIFDIR  = 0o040000
	sIFCHR  = 0o020000
	sIFIFO  = 0o010000

	// digestAlgoSHA256 is PGPHASHALGO_SHA256, the value of FILEDIGESTALGO.
	digestAlgoSHA256 = 8
)

// headerChunk is the serialized payload of one header tag.
type headerChunk struct {
	dataType int32
	count    int32
	align    int
	data     []byte
}

// headerBuilder accumulates tags and serializes them into an rpmdb header
// image (il, dl, index entries, data region — no magic preamble).
type headerBuilder struct {
	chunks map[int]headerChunk
}

func newHeaderBuilder() *headerBuilder {
	return &headerBuilder{chunks: make(map[int]headerChunk)}
}

// addString stores a single NUL-terminated string. Empty values are skipped.
func (b *headerBuilder) addString(tag int, dataType int32, value string) {
	if value == "" {
		return
	}

	b.chunks[tag] = headerChunk{
		dataType: dataType,
		count:    1,
		align:    1,
		data:     append([]byte(value), 0),
	}
}

// addStrings stores a STRING_ARRAY tag. Empty slices are skipped.
func (b *headerBuilder) addStrings(tag int, values []string) {
	if len(values) == 0 {
		return
	}

	var data []byte

	for _, v := range values {
		data = append(data, v...)
		data = append(data, 0)
	}

	b.chunks[tag] = headerChunk{
		dataType: int32(rpmutils.RPM_STRING_ARRAY_TYPE),
		count:    int32(len(values)), //nolint:gosec
		align:    1,
		data:     data,
	}
}

// addInt16s stores an INT16 array (big-endian, 2-byte aligned).
func (b *headerBuilder) addInt16s(tag int, values []uint16) {
	b.addInts(tag, int32(rpmutils.RPM_INT16_TYPE), 2, len(values), func(dst []byte, i int) []byte {
		return binary.BigEndian.AppendUint16(dst, values[i])
	})
}

// addInt32s stores an INT32 array (big-endian, 4-byte aligned).
func (b *headerBuilder) addInt32s(tag int, values []int32) {
	b.addInts(tag, int32(rpmutils.RPM_INT32_TYPE), 4, len(values), func(dst []byte, i int) []byte {
		return binary.BigEndian.AppendUint32(dst, uint32(values[i])) //nolint:gosec
	})
}

// addInt64s stores an INT64 array (big-endian, 8-byte aligned).
func (b *headerBuilder) addInt64s(tag int, values []int64) {
	b.addInts(tag, int32(rpmutils.RPM_INT64_TYPE), 8, len(values), func(dst []byte, i int) []byte {
		return binary.BigEndian.AppendUint64(dst, uint64(values[i])) //nolint:gosec
	})
}

// addInts stores n fixed-width integers appended by put. Empty arrays are
// skipped.
func (b *headerBuilder) addInts(
	tag int, dataType int32, width, n int, put func(dst []byte, i int) []byte,
) {
	if n == 0 {
		return
	}

	data := make([]byte, 0, n*width)
	for i := range n {
		data = put(data, i)
	}

	b.chunks[tag] = headerChunk{
		dataType: dataType,
		count:    int32(n), //nolint:gosec
		align:    width,
		data:     data,
	}
}

// serializeHeader extracts selected tags from an rpmutils.Rpm and serializes
// them into an RPM header image suitable for storage in the rpmdb Packages
// table.
//
// This is a partial serializer — it handles the tags required for rpm -q,
// rpm -qf, and rpm -e to work. Full byte-identity round-trip is not a goal.
func serializeHeader(rpm *rpmutils.Rpm, files []InstalledFile) ([]byte, error) {
	b := newHeaderBuilder()

	addBasicTags(b, rpm)

	for _, g := range []struct {
		name, flags, version int
	}{
		{rpmutils.PROVIDENAME, rpmutils.PROVIDEFLAGS, rpmutils.PROVIDEVERSION},
		{rpmutils.REQUIRENAME, rpmutils.REQUIREFLAGS, rpmutils.REQUIREVERSION},
		{rpmutils.CONFLICTNAME, rpmutils.CONFLICTFLAGS, rpmutils.CONFLICTVERSION},
		{rpmutils.OBSOLETENAME, rpmutils.OBSOLETEFLAGS, rpmutils.OBSOLETEVERSION},
	} {
		addDepGroup(b, rpm, g.name, g.flags, g.version)
	}

	addFileTags(b, files)

	return b.bytes()
}

// addBasicTags writes the scalar metadata tags (name, version, sizes, ...).
func addBasicTags(b *headerBuilder, rpm *rpmutils.Rpm) {
	str := int32(rpmutils.RPM_STRING_TYPE)
	i18n := int32(rpmutils.RPM_I18NSTRING_TYPE)

	for _, st := range []struct {
		tag      int
		dataType int32
	}{
		{rpmutils.NAME, str},
		{rpmutils.VERSION, str},
		{rpmutils.RELEASE, str},
		{rpmutils.ARCH, str},
		{rpmutils.OS, str},
		{rpmutils.SUMMARY, i18n},
		{rpmutils.DESCRIPTION, i18n},
		{rpmutils.LICENSE, str},
		{rpmutils.GROUP, i18n},
		{rpmutils.URL, str},
		{rpmutils.PACKAGER, str},
		{rpmutils.VENDOR, str},
		{rpmutils.BUILDHOST, str},
		{rpmutils.PAYLOADFORMAT, str},
		{rpmutils.PAYLOADCOMPRESSOR, str},
	} {
		v, _ := rpm.Header.GetString(st.tag)
		b.addString(st.tag, st.dataType, v)
	}

	if rpm.Header.HasTag(rpmutils.EPOCH) {
		epoch, _ := rpm.Header.GetInt(rpmutils.EPOCH)
		b.addInt32s(rpmutils.EPOCH, []int32{int32(epoch)}) //nolint:gosec
	}

	buildtime, _ := rpm.Header.GetInt(rpmutils.BUILDTIME)
	b.addInt32s(rpmutils.BUILDTIME, []int32{int32(buildtime)}) //nolint:gosec

	// SIZE is an INT32 tag; payloads that do not fit use LONGSIZE (INT64).
	size, _ := rpm.Header.InstalledSize()
	if size > math.MaxInt32 {
		b.addInt64s(rpmutils.LONGSIZE, []int64{size})
	} else {
		b.addInt32s(rpmutils.SIZE, []int32{int32(size)}) //nolint:gosec
	}
}

// addDepGroup writes a dependency group (names + flags + versions).
func addDepGroup(b *headerBuilder, rpm *rpmutils.Rpm, nameTag, flagsTag, versionTag int) {
	names, _ := rpm.Header.GetStrings(nameTag)
	if len(names) == 0 {
		return
	}

	b.addStrings(nameTag, names)

	flags, _ := rpm.Header.GetUint32s(flagsTag)
	b.addInt32s(flagsTag, toInt32Slice(flags))

	versions, _ := rpm.Header.GetStrings(versionTag)
	b.addStrings(versionTag, versions)
}

// fileColumns holds the per-file parallel arrays of the file metadata tags.
type fileColumns struct {
	basenames   []string
	dirnames    []string
	dirindexes  []int32
	filesizes   []int64
	filemodes   []uint16
	filedigests []string
	filelinktos []string
	fileflags   []int32
	fileusers   []string
	filegroups  []string
	filemtimes  []int32
	hasDigest   bool
	bigFile     bool
}

// buildFileColumns converts InstalledFile records to the parallel arrays
// used by the rpm file tags.
func buildFileColumns(files []InstalledFile) *fileColumns {
	n := len(files)
	c := &fileColumns{
		basenames:   make([]string, n),
		dirindexes:  make([]int32, n),
		filesizes:   make([]int64, n),
		filemodes:   make([]uint16, n),
		filedigests: make([]string, n),
		filelinktos: make([]string, n),
		fileflags:   make([]int32, n),
		fileusers:   make([]string, n),
		filegroups:  make([]string, n),
		filemtimes:  make([]int32, n),
	}

	dirMap := make(map[string]int)

	for i := range files {
		f := &files[i]
		dir := dirFromPath(f.Path)

		idx, ok := dirMap[dir]
		if !ok {
			idx = len(c.dirnames)
			dirMap[dir] = idx
			c.dirnames = append(c.dirnames, dir)
		}

		c.basenames[i] = basenameFromPath(f.Path)
		c.dirindexes[i] = int32(idx) //nolint:gosec
		c.filesizes[i] = f.Size
		c.filemodes[i] = posixFileMode(f.Mode, f.LinkTarget)
		c.filedigests[i] = f.SHA256
		c.filelinktos[i] = f.LinkTarget
		c.fileflags[i] = int32(f.Flags) //nolint:gosec
		c.fileusers[i] = defaultOwner(f.User)
		c.filegroups[i] = defaultOwner(f.Group)
		c.filemtimes[i] = unixTime32(f)

		c.hasDigest = c.hasDigest || f.SHA256 != ""
		c.bigFile = c.bigFile || f.Size > math.MaxInt32
	}

	return c
}

// addFileTags writes the file metadata tags.
func addFileTags(b *headerBuilder, files []InstalledFile) {
	if len(files) == 0 {
		return
	}

	c := buildFileColumns(files)

	b.addStrings(rpmutils.BASENAMES, c.basenames)
	b.addStrings(rpmutils.DIRNAMES, c.dirnames)
	b.addStrings(rpmutils.FILEDIGESTS, c.filedigests)
	b.addStrings(rpmutils.FILELINKTOS, c.filelinktos)
	b.addStrings(rpmutils.FILEUSERNAME, c.fileusers)
	b.addStrings(rpmutils.FILEGROUPNAME, c.filegroups)
	b.addInt32s(rpmutils.DIRINDEXES, c.dirindexes)
	b.addInt16s(rpmutils.FILEMODES, c.filemodes)
	b.addInt32s(rpmutils.FILEFLAGS, c.fileflags)
	b.addInt32s(rpmutils.FILEMTIMES, c.filemtimes)

	// FILESIZES is INT32; any file >= 2 GiB switches the whole column to
	// LONGFILESIZES (INT64), as rpm itself does.
	if c.bigFile {
		b.addInt64s(rpmutils.LONGFILESIZES, c.filesizes)
	} else {
		sizes := make([]int32, len(c.filesizes))
		for i, s := range c.filesizes {
			sizes[i] = int32(s) //nolint:gosec
		}

		b.addInt32s(rpmutils.FILESIZES, sizes)
	}

	if c.hasDigest {
		b.addInt32s(rpmutils.FILEDIGESTALGO, []int32{digestAlgoSHA256})
	}
}

// defaultOwner returns "root" for an unset owner name.
func defaultOwner(name string) string {
	if name == "" {
		return "root"
	}

	return name
}

// unixTime32 returns the file mtime as an rpm INT32 timestamp, or 0 when
// unset or out of range.
func unixTime32(f *InstalledFile) int32 {
	if f.MTime.IsZero() {
		return 0
	}

	ts := f.MTime.Unix()
	if ts < 0 || ts > math.MaxInt32 {
		return 0
	}

	return int32(ts) //nolint:gosec
}

// posixFileMode converts a file mode to the POSIX st_mode value stored in
// RPMTAG_FILEMODES (type bits + setuid/setgid/sticky + permissions). It
// accepts either raw POSIX bits or Go os.FileMode bits (what callers get
// from os.FileInfo.Mode()).
func posixFileMode(mode uint32, linkTarget string) uint16 {
	m := uint16(mode & 0o777)

	fm := os.FileMode(mode)
	if fm&os.ModeSetuid != 0 || mode&0o4000 != 0 {
		m |= 0o4000
	}

	if fm&os.ModeSetgid != 0 || mode&0o2000 != 0 {
		m |= 0o2000
	}

	if fm&os.ModeSticky != 0 || mode&0o1000 != 0 {
		m |= 0o1000
	}

	return m | posixFileType(mode, linkTarget)
}

// posixFileType derives the S_IFMT bits from POSIX type bits, Go mode type
// flags, or (as a last resort) the presence of a symlink target.
func posixFileType(mode uint32, linkTarget string) uint16 {
	if t := mode & sIFMT; t != 0 {
		return uint16(t)
	}

	switch fm := os.FileMode(mode); {
	case fm&os.ModeDir != 0:
		return sIFDIR
	case fm&os.ModeSymlink != 0:
		return sIFLNK
	case fm&os.ModeNamedPipe != 0:
		return sIFIFO
	case fm&os.ModeSocket != 0:
		return sIFSOCK
	case fm&os.ModeCharDevice != 0:
		return sIFCHR
	case fm&os.ModeDevice != 0:
		return sIFBLK
	case linkTarget != "":
		return sIFLNK
	}

	return sIFREG
}

// bytes lays the tags out in ascending tag order — each value aligned to its
// natural boundary — and returns il(4) dl(4) + index entries + data region.
// Unlike an RPM file header there is no magic/reserved preamble, and entry
// offsets are relative to the start of the data region.
func (b *headerBuilder) bytes() ([]byte, error) {
	tags := make([]int, 0, len(b.chunks))
	for tag := range b.chunks {
		tags = append(tags, tag)
	}

	slices.Sort(tags)

	var (
		index bytes.Buffer
		data  bytes.Buffer
	)

	for _, tag := range tags {
		c := b.chunks[tag]

		if pad := data.Len() % c.align; pad != 0 {
			data.Write(make([]byte, c.align-pad))
		}

		e := headerEntry{
			tag:      int32(tag), //nolint:gosec
			dataType: c.dataType,
			offset:   int32(data.Len()), //nolint:gosec
			count:    c.count,
		}
		if err := binary.Write(&index, binary.BigEndian, e); err != nil {
			return nil, err
		}

		data.Write(c.data)
	}

	out := make([]byte, 0, 8+index.Len()+data.Len())
	out = binary.BigEndian.AppendUint32(out, uint32(len(tags)))  //nolint:gosec
	out = binary.BigEndian.AppendUint32(out, uint32(data.Len())) //nolint:gosec
	out = append(out, index.Bytes()...)
	out = append(out, data.Bytes()...)

	return out, nil
}

// headerEntry represents a single index entry of an RPM header.
type headerEntry struct {
	tag      int32
	dataType int32
	offset   int32
	count    int32
}

// Helper functions
func toInt32Slice(u32s []uint32) []int32 {
	result := make([]int32, len(u32s))
	for i, v := range u32s {
		result[i] = int32(v) //nolint:gosec
	}

	return result
}

func dirFromPath(path string) string {
	// Find the last '/'
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}

			return path[:i+1]
		}
	}

	return ""
}

func basenameFromPath(path string) string {
	// Find the last '/'
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}

	return path
}
