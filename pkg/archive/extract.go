package archive

import (
	"context"
	"io"
	"os"

	"github.com/M0Rf30/yap/v2/pkg/buffers"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// extractWithIterator is the shared extraction loop that works with any entryIterator.
// It reads entries from the iterator and writes them to the destination directory,
// honouring the optional pattern filter.
//
//nolint:gocyclo,cyclop // dir + file + symlink dispatch is inherently branchy
func extractWithIterator(
	ctx context.Context,
	it entryIterator,
	destination string,
	patterns []string,
) error {
	defer it.Close()

	dirMap := make(map[string]bool)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		entry, err := it.Next()
		if err == io.EOF { //nolint:errorlint // io.EOF is the documented sentinel
			return nil
		}

		if err != nil {
			return err
		}

		if !matchesAny(patterns, entry.Name) {
			continue
		}

		cleanPath, err := safeJoin(destination, entry.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", entry.Name, "destination", destination)

			return err
		}

		switch {
		case entry.IsDir:
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, 0o755); err != nil { // #nosec G301
				return err
			}

		case entry.IsSymlink:
			// For tar, LinkTarget is set directly. For zip/7z/rar, the target
			// is stored in the file body and needs to be read.
			target := entry.LinkTarget
			if target == "" && entry.Open != nil {
				// Read the symlink target from the file body
				rc, err := entry.Open()
				if err != nil {
					return err
				}

				targetBytes, err := io.ReadAll(rc)
				_ = rc.Close()

				if err != nil {
					return err
				}

				target = string(targetBytes)
			}

			if err := safeSymlinkTarget(entry.Name, target); err != nil {
				logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
					"entry", entry.Name, "target", target)

				return err
			}

			ensureParent(cleanPath, dirMap)
			_ = os.Remove(cleanPath)

			if err := os.Symlink(target, cleanPath); err != nil {
				return err
			}

		default:
			// Regular file
			ensureParent(cleanPath, dirMap)

			if err := writeFileFromEntry(cleanPath, entry); err != nil {
				return err
			}
		}
	}
}

// writeFileFromEntry creates path with the mode from entry and streams the entry body
// from entry.Open().
func writeFileFromEntry(path string, entry archiveEntry) error {
	// Skip rewriting identical files to support resumed extractions.
	if existing, err := os.Stat(path); err == nil && existing.Size() == entry.Size {
		logger.Debug(i18n.T("logger.archive.debug.skip_exists"), "path", path)

		return nil
	}

	// #nosec G304,G703 -- path is constrained by safeJoin to stay inside destination
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm())
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_new_1"),
				"path", path, "error", closeErr)
		}
	}()

	if entry.Open == nil {
		return nil
	}

	rc, err := entry.Open()
	if err != nil {
		return err
	}

	defer func() {
		_ = rc.Close()
	}()

	copyBuf := buffers.DefaultBufferPool.Get().([]byte)
	_, err = io.CopyBuffer(out, rc, copyBuf) //nolint:gosec // size bounded by archive header
	buffers.DefaultBufferPool.Put(copyBuf)   //nolint:staticcheck // SA6002: []byte is fine for sync.Pool

	return err
}
