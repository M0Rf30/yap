// Package archive provides archive creation and manipulation functionality.
package archive

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mholt/archives"

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

// CreateTarCompressed creates a compressed tar archive with the specified
// compression algorithm from the source directory. Supported compression
// algorithms are "zstd", "gzip", and "xz". If compression is empty, defaults
// to "zstd". Returns an error if the compression algorithm is unsupported.
func CreateTarCompressed(
	ctx context.Context,
	sourceDir,
	outputFile,
	compression string,
	formatGNU bool,
) error {
	// Default to zstd if not specified
	if compression == "" {
		compression = "zstd"
	}

	// Map compression string to archives.Compression
	var compressor archives.Compression

	switch compression {
	case "zstd":
		compressor = archives.Zstd{}
	case "gzip":
		compressor = archives.Gz{}
	case "xz":
		compressor = archives.Xz{}
	default:
		return errors.New(
			errors.ErrTypeConfiguration,
			"unsupported compression algorithm",
		).WithContext("compression", compression).
			WithOperation("CreateTarCompressed")
	}

	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	// Retrieve the list of files from the source directory on disk.
	files, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	// Add trailing slashes to directory entries for pacman compatibility
	for i := range files {
		if files[i].IsDir() && !strings.HasSuffix(files[i].NameInArchive, "/") {
			files[i].NameInArchive += "/"
		}
	}

	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.createtarzst.warn.failed_to_close_output_1"),
				"path", cleanFilePath,
				"error", closeErr)
		}
	}()

	format := archives.CompressedArchive{
		Compression: compressor,
		Archival: archives.Tar{
			FormatGNU: formatGNU,
			Uid:       0,
			Gid:       0,
			Uname:     "root",
			Gname:     "root",
		},
	}

	return format.Archive(ctx, out, files)
}

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. This is a convenience wrapper around CreateTarCompressed that
// defaults to zstd compression.
func CreateTarZst(ctx context.Context, sourceDir, outputFile string, formatGNU bool) error {
	return CreateTarCompressed(ctx, sourceDir, outputFile, "zstd", formatGNU)
}

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
	// Reject obvious traversal early — these cannot legitimately appear in
	// any sane archive entry name.
	if strings.Contains(name, "..") {
		// Reject any path segment that is exactly "..". This is stricter
		// than filepath.Clean alone, which could neutralize traversal in
		// some inputs but leaves the intent ambiguous.
		segs := strings.Split(filepath.ToSlash(name), "/")
		if slices.Contains(segs, "..") {
			return "", errors.New(errors.ErrTypePackaging,
				"archive entry contains path traversal").
				WithContext("entry", name).
				WithOperation("safeJoin")
		}
	}

	cleanDest := filepath.Clean(destination)
	joined := filepath.Join(cleanDest, name)
	cleanJoined := filepath.Clean(joined)

	// Special case: destination == "/" is a valid extraction root for
	// ExtractToRoot. filepath.Clean("/") == "/", and we want any path under /
	// to be accepted. Use HasPrefix with a trailing separator to avoid
	// "/etc" matching "/etcfoo".
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

// ExtractFiltered extracts an archive file to the destination directory,
// only including entries whose NameInArchive matches at least one of the
// provided glob patterns (using filepath.Match semantics).  If patterns is
// empty every entry is extracted (equivalent to Extract).
//
//nolint:gocyclo,cyclop // symlink + dir + file handling requires branching; splitting would harm readability
func ExtractFiltered(ctx context.Context, source, destination string, patterns []string) error {
	if len(patterns) == 0 {
		return Extract(ctx, source, destination)
	}

	sourceFile, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.archive.warn.close_failed"), "path", source, "error", closeErr)
		}
	}()

	format, archiveReader, _ := archives.Identify(ctx, "", sourceFile)

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return ErrUnrecognizedArchive
	}

	dirMap := make(map[string]bool)

	return extractor.Extract(
		ctx,
		archiveReader,
		func(_ context.Context, archiveFile archives.FileInfo) error {
			name := archiveFile.NameInArchive

			// Check whether this entry matches any of the caller's patterns.
			matched := false

			for _, pat := range patterns {
				// filepath.Match handles "conf/*" style globs.
				if ok, _ := filepath.Match(pat, name); ok {
					matched = true

					break
				}

				// Also match if the entry is inside a directory that matches.
				if ok, _ := filepath.Match(pat, filepath.Dir(name)); ok {
					matched = true

					break
				}
			}

			if !matched {
				return nil
			}

			cleanNewPath, err := safeJoin(destination, name)
			if err != nil {
				logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
					"entry", name, "destination", destination)

				return err
			}

			if archiveFile.IsDir() {
				dirMap[cleanNewPath] = true

				return os.MkdirAll(cleanNewPath, 0o755) // #nosec
			}

			fileDir := filepath.Dir(cleanNewPath)
			if _, seen := dirMap[fileDir]; !seen {
				dirMap[fileDir] = true

				_ = os.MkdirAll(fileDir, 0o755) // #nosec
			}

			// Handle symlinks: create them instead of trying to open as a regular file.
			if archiveFile.Mode()&os.ModeSymlink != 0 {
				if err := safeSymlinkTarget(archiveFile.LinkTarget); err != nil {
					logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
						"entry", name, "target", archiveFile.LinkTarget)

					return err
				}

				_ = os.Remove(cleanNewPath) // remove stale symlink/file if present

				return os.Symlink(archiveFile.LinkTarget, cleanNewPath)
			}

			// #nosec G304 -- cleanNewPath is constrained by safeJoin to remain
			// inside the destination root; traversal is rejected before this point.
			newFile, err := os.OpenFile(cleanNewPath,
				os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
				archiveFile.Mode())
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := newFile.Close(); closeErr != nil {
					logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
						"path", cleanNewPath,
						"error", closeErr)
				}
			}()

			archiveFileTemp, err := archiveFile.Open()
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := archiveFileTemp.Close(); closeErr != nil {
					logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_archive_1"), "error", closeErr)
				}
			}()

			_, err = io.CopyBuffer(newFile, archiveFileTemp, make([]byte, 32*1024))

			return err
		})
}

// Extract extracts an archive file to the specified destination directory.
// It opens the source archive file, identifies its format, and extracts it to the destination.
//
// Returns an error if there was a problem extracting the files.
func Extract(ctx context.Context, source, destination string) error {
	// Open the source archive file
	sourceFile, err := os.Open(filepath.Clean(source))
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.archive.warn.close_failed"), "path", source, "error", closeErr)
		}
	}()

	// Identify the archive file's format
	format, archiveReader, _ := archives.Identify(ctx, "", sourceFile)

	dirMap := make(map[string]bool)

	// Check if the format is an extractor. If not, return ErrUnrecognizedArchive
	// so callers that expected an archive can detect the no-op case.
	extractor, ok := format.(archives.Extractor)

	if !ok {
		return ErrUnrecognizedArchive
	}

	return extractor.Extract(
		ctx,
		archiveReader,
		func(_ context.Context, archiveFile archives.FileInfo) error {
			fileName := archiveFile.NameInArchive

			cleanNewPath, err := safeJoin(destination, fileName)
			if err != nil {
				logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
					"entry", fileName, "destination", destination)

				return err
			}

			if archiveFile.IsDir() {
				dirMap[cleanNewPath] = true

				return os.MkdirAll(cleanNewPath, 0o755) // #nosec
			}

			fileDir := filepath.Dir(cleanNewPath)
			_, seenDir := dirMap[fileDir]

			if !seenDir {
				dirMap[fileDir] = true

				_ = os.MkdirAll(fileDir, 0o755) // #nosec
			}

			// Handle symlinks: create them instead of trying to open as a regular file.
			if archiveFile.Mode()&os.ModeSymlink != 0 {
				if err := safeSymlinkTarget(archiveFile.LinkTarget); err != nil {
					logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
						"entry", fileName, "target", archiveFile.LinkTarget)

					return err
				}

				_ = os.Remove(cleanNewPath) // remove stale symlink/file if present

				return os.Symlink(archiveFile.LinkTarget, cleanNewPath)
			}

			// Check if file already exists and has the same size to avoid re-extraction
			if existingInfo, err := os.Stat(cleanNewPath); err == nil {
				if existingInfo.Size() == archiveFile.Size() {
					logger.Debug(i18n.T("logger.archive.debug.skip_exists"), "path", cleanNewPath)
					return nil
				}
			}

			// #nosec G304 -- cleanNewPath is constrained by safeJoin to remain
			// inside the destination root; traversal is rejected before this point.
			newFile, err := os.OpenFile(cleanNewPath,
				os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
				archiveFile.Mode())
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := newFile.Close(); closeErr != nil {
					logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
						"path", cleanNewPath,
						"error", closeErr)
				}
			}()

			archiveFileTemp, err := archiveFile.Open()
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := archiveFileTemp.Close(); closeErr != nil {
					logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_archive_1"), "error", closeErr)
				}
			}()

			// Use a buffered copy for better performance on large files
			_, err = io.CopyBuffer(newFile, archiveFileTemp, make([]byte, 32*1024))

			return err
		})
}
