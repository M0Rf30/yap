// Package archive provides archive creation and manipulation functionality.
package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. This consolidates the archive creation logic for YAP packages.
func CreateTarZst(sourceDir, outputFile string, formatGNU bool) error {
	ctx := context.Background()
	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	// Retrieve the list of files from the source directory on disk.
	// The map specifies that the files should be read from the sourceDir
	// and the output path in the archive should be empty.
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
		Compression: archives.Zstd{},
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

// CreateTarGz creates a compressed tar.gz archive from the specified source directory.
func CreateTarGz(sourceDir, outputFile string, formatGNU bool) error {
	ctx := context.Background()
	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	files, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.createtargz.warn.failed_to_close_output_1"),
				"path", cleanFilePath,
				"error", closeErr)
		}
	}()

	format := archives.CompressedArchive{
		Compression: archives.Gz{},
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

// Extract extracts an archive file to the specified destination directory.
// It opens the source archive file, identifies its format, and extracts it to the destination.
//
// Returns an error if there was a problem extracting the files.
func Extract(source, destination string) error {
	ctx := context.Background()

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
