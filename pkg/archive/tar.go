// Package archive provides archive creation and manipulation functionality.
package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. This consolidates the archive creation logic from osutils.
func CreateTarZst(sourceDir, outputFile string, formatGNU bool) error {
	ctx := context.TODO()
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
			logger.Warn("failed to close output file",
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
	ctx := context.TODO()
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
			logger.Warn("failed to close output file",
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
