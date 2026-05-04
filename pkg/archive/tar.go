// Package archive provides archive creation and manipulation functionality.
package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

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

	// Check if the format is an extractor. If not, skip the archive file.
	extractor, ok := format.(archives.Extractor)

	if !ok {
		return nil
	}

	return extractor.Extract(
		ctx,
		archiveReader,
		func(_ context.Context, archiveFile archives.FileInfo) error {
			fileName := archiveFile.NameInArchive
			newPath := filepath.Join(destination, fileName)

			if archiveFile.IsDir() {
				dirMap[newPath] = true

				return os.MkdirAll(newPath, 0o755) // #nosec
			}

			fileDir := filepath.Dir(newPath)
			_, seenDir := dirMap[fileDir]

			if !seenDir {
				dirMap[fileDir] = true

				_ = os.MkdirAll(fileDir, 0o755) // #nosec
			}

			cleanNewPath := filepath.Clean(newPath)

			// Check if file already exists and has the same size to avoid re-extraction
			if existingInfo, err := os.Stat(cleanNewPath); err == nil {
				if existingInfo.Size() == archiveFile.Size() {
					logger.Debug(i18n.T("logger.archive.debug.skip_exists"), "path", cleanNewPath)
					return nil
				}
			}

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
