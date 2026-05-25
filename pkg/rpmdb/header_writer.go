package rpmdb

import (
	"bytes"
	"encoding/binary"

	rpmutils "github.com/sassoftware/go-rpmutils"
)

// headerEntry represents a single tag entry in the RPM header.
type headerEntry struct {
	tag      int32
	dataType int32
	offset   int32
	count    int32
}

// serializeHeader extracts selected tags from an rpmutils.Rpm and serializes
// them into an RPM header blob suitable for storage in the rpmdb.
//
// This is a partial serializer — it handles the tags required for rpm -q,
// rpm -qf, and rpm -e to work. Full byte-identity round-trip is not a goal.
//
//nolint:gocyclo,cyclop // RPM header serialization is inherently a long sequential write of many distinct tag types
func serializeHeader(rpm *rpmutils.Rpm, files []InstalledFile) ([]byte, error) {
	entries := make(map[int]headerEntry)
	blobs := new(bytes.Buffer)

	// Helper to write a string tag
	writeString := func(tag int, value string) error {
		if value == "" {
			return nil
		}

		offset := int32(blobs.Len()) //nolint:gosec

		data := append([]byte(value), 0) // null-terminated
		if _, err := blobs.Write(data); err != nil {
			return err
		}

		entries[tag] = headerEntry{
			tag:      int32(tag), //nolint:gosec
			dataType: int32(rpmutils.RPM_STRING_TYPE),
			offset:   offset,
			count:    1,
		}

		return nil
	}

	// Helper to write a string array tag
	writeStringArray := func(tag int, values []string) error {
		if len(values) == 0 {
			return nil
		}

		offset := int32(blobs.Len()) //nolint:gosec

		for _, v := range values {
			data := append([]byte(v), 0)
			if _, err := blobs.Write(data); err != nil {
				return err
			}
		}

		entries[tag] = headerEntry{
			tag:      int32(tag), //nolint:gosec
			dataType: int32(rpmutils.RPM_STRING_ARRAY_TYPE),
			offset:   offset,
			count:    int32(len(values)), //nolint:gosec
		}

		return nil
	}

	// Helper to write an int32 array tag
	//nolint:dupl // closure parameterized by element type (int32 vs int64)
	writeInt32Array := func(tag int, values []int32) error {
		if len(values) == 0 {
			return nil
		}
		// Align to 4-byte boundary
		if n := blobs.Len() % 4; n != 0 {
			if _, err := blobs.Write(make([]byte, 4-n)); err != nil {
				return err
			}
		}

		offset := int32(blobs.Len()) //nolint:gosec
		for _, v := range values {
			if err := binary.Write(blobs, binary.BigEndian, v); err != nil {
				return err
			}
		}

		entries[tag] = headerEntry{
			tag:      int32(tag), //nolint:gosec
			dataType: int32(rpmutils.RPM_INT32_TYPE),
			offset:   offset,
			count:    int32(len(values)), //nolint:gosec
		}

		return nil
	}

	// Helper to write an int64 array tag
	//nolint:dupl // closure parameterized by element type (int32 vs int64)
	writeInt64Array := func(tag int, values []int64) error {
		if len(values) == 0 {
			return nil
		}
		// Align to 8-byte boundary
		if n := blobs.Len() % 8; n != 0 {
			if _, err := blobs.Write(make([]byte, 8-n)); err != nil {
				return err
			}
		}

		offset := int32(blobs.Len()) //nolint:gosec
		for _, v := range values {
			if err := binary.Write(blobs, binary.BigEndian, v); err != nil {
				return err
			}
		}

		entries[tag] = headerEntry{
			tag:      int32(tag), //nolint:gosec
			dataType: int32(rpmutils.RPM_INT64_TYPE),
			offset:   offset,
			count:    int32(len(values)), //nolint:gosec
		}

		return nil
	}

	// Extract basic metadata
	name, _ := rpm.Header.GetString(rpmutils.NAME)
	version, _ := rpm.Header.GetString(rpmutils.VERSION)
	release, _ := rpm.Header.GetString(rpmutils.RELEASE)
	arch, _ := rpm.Header.GetString(rpmutils.ARCH)
	os, _ := rpm.Header.GetString(rpmutils.OS)
	summary, _ := rpm.Header.GetString(rpmutils.SUMMARY)
	description, _ := rpm.Header.GetString(rpmutils.DESCRIPTION)
	license, _ := rpm.Header.GetString(rpmutils.LICENSE)
	group, _ := rpm.Header.GetString(rpmutils.GROUP)
	url, _ := rpm.Header.GetString(rpmutils.URL)
	packager, _ := rpm.Header.GetString(rpmutils.PACKAGER)
	vendor, _ := rpm.Header.GetString(rpmutils.VENDOR)
	buildhost, _ := rpm.Header.GetString(rpmutils.BUILDHOST)

	// Table-driven basic string tags
	stringTags := []struct {
		tag   int
		value string
	}{
		{rpmutils.NAME, name},
		{rpmutils.VERSION, version},
		{rpmutils.RELEASE, release},
		{rpmutils.ARCH, arch},
		{rpmutils.OS, os},
		{rpmutils.SUMMARY, summary},
		{rpmutils.DESCRIPTION, description},
		{rpmutils.LICENSE, license},
		{rpmutils.GROUP, group},
		{rpmutils.URL, url},
		{rpmutils.PACKAGER, packager},
		{rpmutils.VENDOR, vendor},
		{rpmutils.BUILDHOST, buildhost},
	}

	for _, st := range stringTags {
		if err := writeString(st.tag, st.value); err != nil {
			return nil, err
		}
	}

	// EPOCH (optional)
	if rpm.Header.HasTag(rpmutils.EPOCH) {
		epoch, _ := rpm.Header.GetInt(rpmutils.EPOCH)
		if err := writeInt32Array(rpmutils.EPOCH, []int32{int32(epoch)}); err != nil { //nolint:gosec
			return nil, err
		}
	}

	// BUILDTIME
	buildtime, _ := rpm.Header.GetInt(rpmutils.BUILDTIME)

	bt := int32(buildtime) //nolint:gosec
	if err := writeInt32Array(rpmutils.BUILDTIME, []int32{bt}); err != nil {
		return nil, err
	}

	// SIZE (uncompressed payload size)
	size, _ := rpm.Header.InstalledSize()
	if err := writeInt64Array(rpmutils.SIZE, []int64{size}); err != nil {
		return nil, err
	}

	// Helper to write a dependency group (name + flags + versions)
	writeDepGroup := func(
		nameTag, flagsTag, versionTag int,
		names []string,
	) error {
		if err := writeStringArray(nameTag, names); err != nil {
			return err
		}

		if len(names) > 0 {
			flags, _ := rpm.Header.GetUint32s(flagsTag)
			if err := writeInt32Array(flagsTag, toInt32Slice(flags)); err != nil {
				return err
			}

			versions, _ := rpm.Header.GetStrings(versionTag)
			if err := writeStringArray(versionTag, versions); err != nil {
				return err
			}
		}

		return nil
	}

	// PROVIDES
	provides, _ := rpm.Header.GetStrings(rpmutils.PROVIDENAME)
	if err := writeDepGroup(
		rpmutils.PROVIDENAME,
		rpmutils.PROVIDEFLAGS,
		rpmutils.PROVIDEVERSION,
		provides,
	); err != nil {
		return nil, err
	}

	// REQUIRES
	requires, _ := rpm.Header.GetStrings(rpmutils.REQUIRENAME)
	if err := writeDepGroup(
		rpmutils.REQUIRENAME,
		rpmutils.REQUIREFLAGS,
		rpmutils.REQUIREVERSION,
		requires,
	); err != nil {
		return nil, err
	}

	// CONFLICTS
	conflicts, _ := rpm.Header.GetStrings(rpmutils.CONFLICTNAME)
	if err := writeDepGroup(
		rpmutils.CONFLICTNAME,
		rpmutils.CONFLICTFLAGS,
		rpmutils.CONFLICTVERSION,
		conflicts,
	); err != nil {
		return nil, err
	}

	// OBSOLETES
	obsoletes, _ := rpm.Header.GetStrings(rpmutils.OBSOLETENAME)
	if err := writeDepGroup(
		rpmutils.OBSOLETENAME,
		rpmutils.OBSOLETEFLAGS,
		rpmutils.OBSOLETEVERSION,
		obsoletes,
	); err != nil {
		return nil, err
	}

	// FILE METADATA
	if len(files) > 0 { //nolint:nestif // file metadata block is sequential array population
		basenames := make([]string, len(files))
		dirnames := make([]string, 0)
		dirindexes := make([]int32, len(files))
		filesizes := make([]int64, len(files))
		filemodes := make([]int32, len(files))
		filedigests := make([]string, len(files))
		filelinktos := make([]string, len(files))
		fileflags := make([]int32, len(files))
		fileusers := make([]string, len(files))
		filegroups := make([]string, len(files))
		filemtimes := make([]int32, len(files))

		// Build directory list and indices
		dirMap := make(map[string]int)

		for i := range files {
			f := &files[i]
			dir := dirFromPath(f.Path)

			if _, ok := dirMap[dir]; !ok {
				dirMap[dir] = len(dirnames)
				dirnames = append(dirnames, dir)
			}
		}

		// Populate file arrays
		for i := range files {
			f := &files[i]
			basenames[i] = basenameFromPath(f.Path)
			dirindexes[i] = int32(dirMap[dirFromPath(f.Path)]) //nolint:gosec
			filesizes[i] = f.Size
			filemodes[i] = int32(f.Mode) //nolint:gosec
			filedigests[i] = f.SHA256
			filelinktos[i] = f.LinkTarget
			fileflags[i] = int32(f.Flags) //nolint:gosec
			fileusers[i] = f.User
			filegroups[i] = f.Group
			filemtimes[i] = int32(f.MTime.Unix()) //nolint:gosec
		}

		// Table-driven file metadata tags
		fileStringTags := []struct {
			tag    int
			values []string
		}{
			{rpmutils.BASENAMES, basenames},
			{rpmutils.DIRNAMES, dirnames},
			{rpmutils.FILEDIGESTS, filedigests},
			{rpmutils.FILELINKTOS, filelinktos},
			{rpmutils.FILEUSERNAME, fileusers},
			{rpmutils.FILEGROUPNAME, filegroups},
		}

		for _, fst := range fileStringTags {
			if err := writeStringArray(fst.tag, fst.values); err != nil {
				return nil, err
			}
		}

		fileInt32Tags := []struct {
			tag    int
			values []int32
		}{
			{rpmutils.DIRINDEXES, dirindexes},
			{rpmutils.FILEMODES, filemodes},
			{rpmutils.FILEFLAGS, fileflags},
			{rpmutils.FILEMTIMES, filemtimes},
		}

		for _, fit := range fileInt32Tags {
			if err := writeInt32Array(fit.tag, fit.values); err != nil {
				return nil, err
			}
		}

		if err := writeInt64Array(rpmutils.FILESIZES, filesizes); err != nil {
			return nil, err
		}
	}

	// PAYLOAD FORMAT
	payloadFormat, _ := rpm.Header.GetString(rpmutils.PAYLOADFORMAT)
	if err := writeString(rpmutils.PAYLOADFORMAT, payloadFormat); err != nil {
		return nil, err
	}

	// PAYLOAD COMPRESSOR
	payloadCompressor, _ := rpm.Header.GetString(rpmutils.PAYLOADCOMPRESSOR)
	if err := writeString(rpmutils.PAYLOADCOMPRESSOR, payloadCompressor); err != nil {
		return nil, err
	}

	// Serialize header blob
	return serializeHeaderBlob(entries, blobs)
}

// serializeHeaderBlob writes the header intro, index entries, and data blob.
func serializeHeaderBlob(entries map[int]headerEntry, blobs *bytes.Buffer) ([]byte, error) {
	// Sort entries by tag
	var tags []int
	for tag := range entries {
		tags = append(tags, tag)
	}
	// Simple bubble sort for small sets
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[j] < tags[i] {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}

	// Write header intro
	result := new(bytes.Buffer)

	intro := struct {
		Magic    [8]byte
		Reserved [4]byte
		Entries  uint32
		Size     uint32
	}{
		Magic:   [8]byte{0x8e, 0xad, 0xe8, 0x01, 0x00, 0x00, 0x00, 0x00},
		Entries: uint32(len(tags)),   //nolint:gosec
		Size:    uint32(blobs.Len()), //nolint:gosec
	}
	if err := binary.Write(result, binary.BigEndian, intro); err != nil {
		return nil, err
	}

	// Write index entries
	for _, tag := range tags {
		e := entries[tag]
		if err := binary.Write(result, binary.BigEndian, e); err != nil {
			return nil, err
		}
	}

	// Write data blob
	if _, err := result.Write(blobs.Bytes()); err != nil {
		return nil, err
	}

	return result.Bytes(), nil
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
