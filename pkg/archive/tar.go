// Package archive provides archive creation and manipulation functionality.
//
// This package implements a focused subset of tar/zip handling needed by YAP:
//
//   - Create compressed tar archives (.tar.zst / .tar.gz / .tar.xz) used as
//     intermediate artifacts and as the on-disk format for Pacman / APK
//     packages.
//   - Extract a small set of recognized archive types to disk: tar (plain or
//     wrapped in gzip / zstd / xz / bz2) and zip. Other formats (rar, 7z, …)
//     return the ErrUnrecognizedArchive sentinel so callers can fall through
//     to a system binary or treat it as a no-op.
//
// All extraction paths defend against zip-slip path traversal via safeJoin
// and against malicious symlink targets via safeSymlinkTarget.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bodgit/sevenzip"
	kzstd "github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ErrUnrecognizedArchive is returned by Extract / ExtractFiltered when the
// input file's format is not recognized as an extractable archive. Callers
// that legitimately accept non-archive inputs (e.g. plain patch files in
// pkg/source.Source.Get) can detect this sentinel and treat it as a no-op;
// callers that expect a real archive should treat it as a hard error.
var ErrUnrecognizedArchive = stderrors.New("archive format not recognized")

// magic-byte sniff buffer size — large enough for the longest signature
// we check (xz: 6 bytes, zip: 4 bytes, zstd: 4 bytes, gzip: 2 bytes,
// bz2: 3 bytes, tar ustar magic at offset 257 → need 265).
const sniffSize = 265

// archiveKind identifies the detected archive container.
type archiveKind int

const (
	kindUnknown archiveKind = iota
	kindTar
	kindTarGz
	kindTarZst
	kindTarXz
	kindTarBz2
	kindZip
	kind7z
	kindRar
)

// detectKind sniffs the first sniffSize bytes of an archive file and combines
// that with the filename suffix to classify it. The returned io.Reader is
// the original file wrapped so the consumed magic bytes are still available
// to downstream decoders.
func detectKind(f *os.File, name string) (archiveKind, io.Reader, error) {
	header := make([]byte, sniffSize)

	n, err := io.ReadFull(f, header)
	// io.ReadFull returns io.ErrUnexpectedEOF for short reads (file smaller
	// than sniffSize) and io.EOF for empty files. Both are acceptable here —
	// we'll classify whatever bytes we did read.
	//
	//nolint:errorlint // direct sentinel comparison is the documented pattern for io.ReadFull
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return kindUnknown, nil, err
	}

	header = header[:n]

	// Rewind via a MultiReader of (header || rest-of-file) so the chosen
	// decoder sees the full byte stream.
	reader := io.MultiReader(strings.NewReader(string(header)), f)

	kind := classify(header, name)
	if kind == kindUnknown {
		return kindUnknown, reader, ErrUnrecognizedArchive
	}

	return kind, reader, nil
}

// classify inspects magic bytes (primary) with filename hints as a tiebreaker
// for tar (which has no magic at offset 0).
func classify(header []byte, name string) archiveKind {
	switch {
	case hasPrefix(header, []byte{0x1f, 0x8b}):
		return kindTarGz
	case hasPrefix(header, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return kindTarZst
	case hasPrefix(header, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		return kindTarXz
	case hasPrefix(header, []byte{'B', 'Z', 'h'}):
		return kindTarBz2
	case hasPrefix(header, []byte{'P', 'K', 0x03, 0x04}),
		hasPrefix(header, []byte{'P', 'K', 0x05, 0x06}),
		hasPrefix(header, []byte{'P', 'K', 0x07, 0x08}):
		return kindZip

	// 7z magic: "7z" + 0xBC 0xAF 0x27 0x1C
	case hasPrefix(header, []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}):
		return kind7z

	// Rar v1.5+ magic: "Rar!" + 0x1A 0x07 0x00
	// Rar v5  magic: "Rar!" + 0x1A 0x07 0x01 0x00
	case hasPrefix(header, []byte{'R', 'a', 'r', '!', 0x1A, 0x07}):
		return kindRar
	}

	// ustar magic at offset 257 marks a plain (uncompressed) tar.
	if len(header) >= 265 && string(header[257:262]) == "ustar" {
		return kindTar
	}

	// Suffix fallback is restricted to plain ".tar" — the compressed
	// variants have unambiguous magic bytes at offset 0, so a missing
	// magic AND a misleading suffix means the file is not what it claims
	// to be (e.g. a placeholder dropped by a test or a corrupt download).
	// We return kindUnknown in that case so callers see ErrUnrecognizedArchive
	// rather than a downstream "gzip: invalid header"-style decoder error.
	if strings.HasSuffix(strings.ToLower(name), ".tar") {
		return kindTar
	}

	return kindUnknown
}

func hasPrefix(buf, prefix []byte) bool {
	return len(buf) >= len(prefix) && bytes.Equal(buf[:len(prefix)], prefix)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateTarCompressed creates a compressed tar archive with the specified
// compression algorithm from the source directory. Supported compression
// algorithms are "zstd", "gzip", and "xz". If compression is empty, defaults
// to "zstd". Returns an error if the compression algorithm is unsupported.
//
// formatGNU selects tar.FormatGNU when true (used for Pacman packages); the
// default is tar.FormatPAX (used for APK and source bundles).
func CreateTarCompressed(
	ctx context.Context,
	sourceDir,
	outputFile,
	compression string,
	formatGNU bool,
) error {
	if compression == "" {
		compression = "zstd"
	}

	cleanOut := filepath.Clean(outputFile)

	out, err := os.Create(cleanOut)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.createtarzst.warn.failed_to_close_output_1"),
				"path", cleanOut,
				"error", closeErr)
		}
	}()

	cw, closeCompressor, err := newCompressedWriter(out, compression)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(cw)

	if err := writeTarFromDir(ctx, tw, sourceDir, formatGNU); err != nil {
		_ = tw.Close()
		_ = closeCompressor()

		return err
	}

	if err := tw.Close(); err != nil {
		_ = closeCompressor()

		return err
	}

	return closeCompressor()
}

// newCompressedWriter returns an io.Writer that streams compressed bytes to
// dst, plus a close function that finalises the compressor.
func newCompressedWriter(dst io.Writer, compression string) (io.Writer, func() error, error) {
	switch compression {
	case "zstd":
		zw, err := kzstd.NewWriter(dst)
		if err != nil {
			return nil, nil, err
		}

		return zw, zw.Close, nil

	case "gzip":
		gw := gzip.NewWriter(dst)

		return gw, gw.Close, nil

	case "xz":
		xw, err := xz.NewWriter(dst)
		if err != nil {
			return nil, nil, err
		}

		return xw, xw.Close, nil

	default:
		return nil, nil, errors.New(
			errors.ErrTypeConfiguration,
			"unsupported compression algorithm",
		).WithContext("compression", compression).
			WithOperation("CreateTarCompressed")
	}
}

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. This is a convenience wrapper around CreateTarCompressed that
// defaults to zstd compression.
func CreateTarZst(ctx context.Context, sourceDir, outputFile string, formatGNU bool) error {
	return CreateTarCompressed(ctx, sourceDir, outputFile, "zstd", formatGNU)
}

// writeTarFromDir walks sourceDir and writes every entry into tw with names
// relative to sourceDir. Honours symlinks. Mirrors the previous behaviour
// of archives.FilesFromDisk (FollowSymlinks: false).
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func writeTarFromDir(ctx context.Context, tw *tar.Writer, sourceDir string, formatGNU bool) error {
	sourceDir = filepath.Clean(sourceDir)

	tarFormat := tar.FormatPAX
	if formatGNU {
		tarFormat = tar.FormatGNU
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		// Skip the source directory itself.
		if path == sourceDir {
			return nil
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		nameInArchive := filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		hdr, err := buildTarHeader(path, nameInArchive, info)
		if err != nil {
			return err
		}

		hdr.Format = tarFormat

		// Pacman expects directory entries to have a trailing slash.
		if d.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		// Only regular files have body content.
		if info.Mode().IsRegular() {
			if err := copyRegularFile(path, tw); err != nil {
				return err
			}
		}

		return nil
	})
}

// buildTarHeader constructs a tar.Header for the given on-disk path. Symlinks
// are recorded with their link target; ownership is forced to root:root for
// reproducible package builds (mirrors the previous archives.Tar config).
func buildTarHeader(path, nameInArchive string, info fs.FileInfo) (*tar.Header, error) {
	var linkTarget string

	if info.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}

		linkTarget = t
	}

	hdr, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return nil, err
	}

	hdr.Name = nameInArchive
	hdr.Uid = 0
	hdr.Gid = 0
	hdr.Uname = "root"
	hdr.Gname = "root"

	return hdr, nil
}

// copyRegularFile streams the contents of path into tw, using a buffered copy.
func copyRegularFile(path string, tw io.Writer) error {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path is walked from a caller-supplied build directory
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	_, err = io.CopyBuffer(tw, f, make([]byte, 32*1024))

	return err
}

// ---------------------------------------------------------------------------
// SafeJoin / SafeSymlinkTarget
// ---------------------------------------------------------------------------

// SafeJoin is the exported wrapper around safeJoin, exposed for callers in
// sibling packages (e.g. pkg/builders/common.extractAPK) that need the same
// containment guarantee when extracting custom archive formats.
func SafeJoin(destination, name string) (string, error) {
	return safeJoin(destination, name)
}

// SafeSymlinkTarget is the exported wrapper around safeSymlinkTarget.
func SafeSymlinkTarget(target string) error {
	return safeSymlinkTarget(target)
}

// safeJoin joins destination + name and verifies the result stays inside
// destination. Defends against "zip-slip" / path-traversal attacks where an
// archive entry name contains "../" or an absolute path that would escape the
// extraction root.
//
// Returns the cleaned absolute-ish path on success, or an error if the entry
// would escape destination.
func safeJoin(destination, name string) (string, error) {
	if strings.Contains(name, "..") {
		segs := strings.Split(filepath.ToSlash(name), "/")
		if slices.Contains(segs, "..") {
			return "", errors.New(errors.ErrTypePackaging,
				"archive entry contains path traversal").
				WithContext("entry", name).
				WithOperation("safeJoin")
		}
	}

	cleanDest := filepath.Clean(destination)
	cleanJoined := filepath.Clean(filepath.Join(cleanDest, name))

	prefix := cleanDest
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}

	if cleanJoined != cleanDest && !strings.HasPrefix(cleanJoined, prefix) {
		return "", errors.New(errors.ErrTypePackaging,
			"archive entry escapes destination").
			WithContext("entry", name).
			WithContext("destination", cleanDest).
			WithContext("resolved", cleanJoined).
			WithOperation("safeJoin")
	}

	return cleanJoined, nil
}

// safeSymlinkTarget validates a symlink target before it is created on disk.
// Absolute targets and targets containing ".." segments are rejected because
// they would let an attacker plant a link that, when later dereferenced by
// build scripts or by ExtractToRoot's follow-up writes, redirects to arbitrary
// filesystem locations.
func safeSymlinkTarget(target string) error {
	if target == "" {
		return nil
	}

	if filepath.IsAbs(target) {
		return errors.New(errors.ErrTypePackaging,
			"symlink target is absolute").
			WithContext("target", target).
			WithOperation("safeSymlinkTarget")
	}

	if slices.Contains(strings.Split(filepath.ToSlash(target), "/"), "..") {
		return errors.New(errors.ErrTypePackaging,
			"symlink target contains traversal").
			WithContext("target", target).
			WithOperation("safeSymlinkTarget")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Extract
// ---------------------------------------------------------------------------

// Extract extracts an archive file to the specified destination directory.
// Supported formats: tar (plain or gzip / zstd / xz / bz2 wrapped) and zip.
// Returns ErrUnrecognizedArchive for any other input.
func Extract(ctx context.Context, source, destination string) error {
	return ExtractFiltered(ctx, source, destination, nil)
}

// ExtractFiltered extracts an archive file to the destination directory, only
// including entries whose name matches one of the provided glob patterns
// (filepath.Match semantics). If patterns is empty every entry is extracted.
func ExtractFiltered(ctx context.Context, source, destination string, patterns []string) error {
	sourceFile, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.archive.warn.close_failed"), "path", source, "error", closeErr)
		}
	}()

	kind, reader, err := detectKind(sourceFile, source)
	if err != nil {
		return err
	}

	switch kind {
	case kindZip:
		return extractZip(ctx, sourceFile, destination, patterns)
	case kind7z:
		return extract7z(ctx, sourceFile, destination, patterns)
	case kindRar:
		return extractRar(ctx, reader, destination, patterns)
	}

	tarReader, closeFn, err := openTarStream(reader, kind)
	if err != nil {
		return err
	}

	defer func() {
		if closeFn != nil {
			_ = closeFn()
		}
	}()

	return extractTar(ctx, tarReader, destination, patterns)
}

// openTarStream wraps reader in the appropriate decompressor for the given
// tar variant. For kindTar the reader is returned unchanged.
func openTarStream(reader io.Reader, kind archiveKind) (*tar.Reader, func() error, error) {
	switch kind {
	case kindTar:
		return tar.NewReader(reader), nil, nil

	case kindTarGz:
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, nil, err
		}

		return tar.NewReader(gr), gr.Close, nil

	case kindTarZst:
		zr, err := kzstd.NewReader(reader)
		if err != nil {
			return nil, nil, err
		}

		return tar.NewReader(zr), func() error { zr.Close(); return nil }, nil

	case kindTarXz:
		xr, err := xz.NewReader(reader)
		if err != nil {
			return nil, nil, err
		}

		return tar.NewReader(xr), nil, nil

	case kindTarBz2:
		// compress/bzip2 has no Close method.
		return tar.NewReader(bzip2.NewReader(reader)), nil, nil

	default:
		return nil, nil, fmt.Errorf("unsupported archive kind: %d", kind)
	}
}

// matchesAny reports whether name matches at least one of patterns. An empty
// patterns slice matches everything.
func matchesAny(patterns []string, name string) bool {
	if len(patterns) == 0 {
		return true
	}

	for _, pat := range patterns {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}

		if ok, _ := filepath.Match(pat, filepath.Dir(name)); ok {
			return true
		}
	}

	return false
}

// extractTar reads tar entries from tr and writes them under destination,
// honouring the optional pattern filter.
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func extractTar(ctx context.Context, tr *tar.Reader, destination string, patterns []string) error {
	dirMap := make(map[string]bool)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		hdr, err := tr.Next()
		if err == io.EOF { //nolint:errorlint // io.EOF is the documented sentinel
			return nil
		}

		if err != nil {
			return err
		}

		if !matchesAny(patterns, hdr.Name) {
			continue
		}

		cleanPath, err := safeJoin(destination, hdr.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", hdr.Name, "destination", destination)

			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, 0o755); err != nil { // #nosec G301
				return err
			}

		case tar.TypeSymlink:
			if err := safeSymlinkTarget(hdr.Linkname); err != nil {
				logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
					"entry", hdr.Name, "target", hdr.Linkname)

				return err
			}

			ensureParent(cleanPath, dirMap)
			_ = os.Remove(cleanPath)

			if err := os.Symlink(hdr.Linkname, cleanPath); err != nil {
				return err
			}

		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA appears in old tarballs
			ensureParent(cleanPath, dirMap)

			if err := writeTarFile(cleanPath, hdr, tr); err != nil {
				return err
			}

		default:
			// Skip FIFOs, hardlinks (rare in source tarballs), char/block
			// devices, etc. — neither produced by YAP nor safe to materialise
			// in a build directory.
			continue
		}
	}
}

// ensureParent creates the parent directory of path if not already seen.
func ensureParent(path string, dirMap map[string]bool) {
	parent := filepath.Dir(path)
	if _, seen := dirMap[parent]; !seen {
		dirMap[parent] = true
		_ = os.MkdirAll(parent, 0o755) // #nosec G301 -- intermediate dirs need read+exec
	}
}

// writeTarFile creates path with the mode from hdr and streams the entry body
// from tr.
func writeTarFile(path string, hdr *tar.Header, tr io.Reader) error {
	// Skip rewriting identical files to support resumed extractions.
	if existing, err := os.Stat(path); err == nil && existing.Size() == hdr.Size {
		logger.Debug(i18n.T("logger.archive.debug.skip_exists"), "path", path)

		return nil
	}

	// #nosec G304,G703 -- path is constrained by safeJoin to stay inside destination
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
				"path", path, "error", closeErr)
		}
	}()

	_, err = io.CopyBuffer(out, tr, make([]byte, 32*1024)) //nolint:gosec // size bounded by tar header

	return err
}

// extractZip handles .zip / .jar inputs. The zip reader needs random access,
// so we use *os.File directly (the read offset is reset before use).
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func extractZip(ctx context.Context, f *os.File, destination string, patterns []string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return err
	}

	dirMap := make(map[string]bool)

	for _, zf := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !matchesAny(patterns, zf.Name) {
			continue
		}

		cleanPath, err := safeJoin(destination, zf.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", zf.Name, "destination", destination)

			return err
		}

		mode := zf.Mode()

		switch {
		case mode.IsDir():
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, 0o755); err != nil { // #nosec G301
				return err
			}

		case mode&os.ModeSymlink != 0:
			if err := writeZipSymlink(zf, cleanPath, dirMap); err != nil {
				return err
			}

		default:
			ensureParent(cleanPath, dirMap)

			if err := writeZipFile(zf, cleanPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeSymlinkFromOpener resolves a symlink target by reading the entry body
// produced by openFn and creates the symlink at cleanPath after validating
// the target. Used by both the zip and 7z extractors, which share this exact
// shape (a file-like entry whose Open() yields the link target as the body).
func writeSymlinkFromOpener(
	entryName, cleanPath string,
	openFn func() (io.ReadCloser, error),
	dirMap map[string]bool,
) error {
	rc, err := openFn()
	if err != nil {
		return err
	}

	target, err := io.ReadAll(rc)
	_ = rc.Close()

	if err != nil {
		return err
	}

	if err := safeSymlinkTarget(string(target)); err != nil {
		logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
			"entry", entryName, "target", string(target))

		return err
	}

	ensureParent(cleanPath, dirMap)
	_ = os.Remove(cleanPath)

	return os.Symlink(string(target), cleanPath)
}

// writeZipSymlink resolves the symlink target stored in the zip entry body
// and creates the symlink, applying the standard target validation.
func writeZipSymlink(zf *zip.File, cleanPath string, dirMap map[string]bool) error {
	return writeSymlinkFromOpener(zf.Name, cleanPath, zf.Open, dirMap)
}

// writeZipFile streams a single regular-file zip entry to disk.
func writeZipFile(zf *zip.File, cleanPath string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}

	defer func() {
		_ = rc.Close()
	}()

	// #nosec G304,G703 -- cleanPath is constrained by safeJoin
	out, err := os.OpenFile(cleanPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		zf.Mode().Perm())
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
				"path", cleanPath, "error", closeErr)
		}
	}()

	// #nosec G110 -- size bounded by zip header; YAP processes vendor-provided
	// sources, not adversarial uploads.
	_, err = io.CopyBuffer(out, rc, make([]byte, 32*1024))

	return err
}

// ---------------------------------------------------------------------------
// 7z (bodgit/sevenzip)
// ---------------------------------------------------------------------------

// extract7z reads .7z archives. sevenzip requires random access (io.ReaderAt
// + size), so we operate on the underlying *os.File rather than the sniffed
// MultiReader. The file offset is reset before use.
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func extract7z(ctx context.Context, f *os.File, destination string, patterns []string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	zr, err := sevenzip.NewReader(f, info.Size())
	if err != nil {
		return err
	}

	dirMap := make(map[string]bool)

	for _, sf := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !matchesAny(patterns, sf.Name) {
			continue
		}

		cleanPath, err := safeJoin(destination, sf.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", sf.Name, "destination", destination)

			return err
		}

		mode := sf.FileInfo().Mode()

		switch {
		case mode.IsDir():
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, 0o755); err != nil { // #nosec G301
				return err
			}

		case mode&os.ModeSymlink != 0:
			if err := write7zSymlink(sf, cleanPath, dirMap); err != nil {
				return err
			}

		default:
			ensureParent(cleanPath, dirMap)

			if err := write7zFile(sf, cleanPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// write7zSymlink reads the symlink target from the 7z entry body and creates
// the symlink, applying the standard target validation.
func write7zSymlink(sf *sevenzip.File, cleanPath string, dirMap map[string]bool) error {
	return writeSymlinkFromOpener(sf.Name, cleanPath, sf.Open, dirMap)
}

// write7zFile streams a single regular-file 7z entry to disk.
func write7zFile(sf *sevenzip.File, cleanPath string) error {
	rc, err := sf.Open()
	if err != nil {
		return err
	}

	defer func() {
		_ = rc.Close()
	}()

	// #nosec G304,G703 -- cleanPath is constrained by safeJoin
	out, err := os.OpenFile(cleanPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		sf.FileInfo().Mode().Perm())
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
				"path", cleanPath, "error", closeErr)
		}
	}()

	// #nosec G110 -- size bounded by 7z header; YAP processes vendor-provided
	// sources, not adversarial uploads.
	_, err = io.CopyBuffer(out, rc, make([]byte, 32*1024))

	return err
}

// ---------------------------------------------------------------------------
// rar (nwaples/rardecode)
// ---------------------------------------------------------------------------

// extractRar reads .rar archives. rardecode operates on an io.Reader so the
// sniffed MultiReader is fine.
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func extractRar(ctx context.Context, reader io.Reader, destination string, patterns []string) error {
	rr, err := rardecode.NewReader(reader)
	if err != nil {
		return err
	}

	dirMap := make(map[string]bool)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		hdr, err := rr.Next()
		if err == io.EOF { //nolint:errorlint // io.EOF is the documented sentinel
			return nil
		}

		if err != nil {
			return err
		}

		if !matchesAny(patterns, hdr.Name) {
			continue
		}

		cleanPath, err := safeJoin(destination, hdr.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", hdr.Name, "destination", destination)

			return err
		}

		mode := hdr.Mode()

		switch {
		case hdr.IsDir:
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, 0o755); err != nil { // #nosec G301
				return err
			}

		case mode&os.ModeSymlink != 0:
			if err := writeRarSymlink(rr, hdr, cleanPath, dirMap); err != nil {
				return err
			}

		default:
			ensureParent(cleanPath, dirMap)

			if err := writeRarFile(rr, hdr, cleanPath); err != nil {
				return err
			}
		}
	}
}

// writeRarSymlink reads the symlink target from the rar entry body and creates
// the symlink, applying the standard target validation.
func writeRarSymlink(
	rr *rardecode.Reader, hdr *rardecode.FileHeader, cleanPath string, dirMap map[string]bool,
) error {
	target, err := io.ReadAll(rr)
	if err != nil {
		return err
	}

	if err := safeSymlinkTarget(string(target)); err != nil {
		logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
			"entry", hdr.Name, "target", string(target))

		return err
	}

	ensureParent(cleanPath, dirMap)
	_ = os.Remove(cleanPath)

	return os.Symlink(string(target), cleanPath)
}

// writeRarFile streams a single regular-file rar entry to disk.
func writeRarFile(rr *rardecode.Reader, hdr *rardecode.FileHeader, cleanPath string) error {
	// #nosec G304,G703 -- cleanPath is constrained by safeJoin
	out, err := os.OpenFile(cleanPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		hdr.Mode().Perm())
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
				"path", cleanPath, "error", closeErr)
		}
	}()

	// #nosec G110 -- size bounded by rar header; YAP processes vendor-provided
	// sources, not adversarial uploads.
	_, err = io.CopyBuffer(out, rr, make([]byte, 32*1024))

	return err
}
