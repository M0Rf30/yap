package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/buffers"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/safepath"
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
	defer func() { _ = it.Close() }()

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

		if entry.Skip || !matchesAny(patterns, entry.Name) {
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
			dirPath, err := resolveWithin(destination, cleanPath, true)
			if err != nil {
				return err
			}

			if err := os.MkdirAll(dirPath, 0o755); err != nil {
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

			linkPath, err := prepareTarget(destination, cleanPath)
			if err != nil {
				return err
			}

			_ = os.Remove(linkPath)

			if err := os.Symlink(target, linkPath); err != nil {
				return err
			}

		case entry.IsHardlink:
			if err := extractHardlink(destination, cleanPath, entry.LinkTarget); err != nil {
				return err
			}

		default:
			// Regular file
			filePath, err := prepareTarget(destination, cleanPath)
			if err != nil {
				return err
			}

			if err := writeFileFromEntry(filePath, &entry); err != nil {
				return err
			}
		}
	}
}

// writeFileFromEntry creates path with the mode from entry and streams the entry body
// from entry.Open().
func writeFileFromEntry(path string, entry *archiveEntry) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode.Perm()) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.archive.warn.failed_to_close_new"),
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

// resolveWithin maps cleanPath (lexically inside destination) to a host path that is
// guaranteed to stay inside destination even when earlier entries planted symlinks.
// When full is false the final path component is not resolved, so an existing symlink
// there can be replaced instead of followed. destination "/" keeps lexical behaviour.
func resolveWithin(destination, cleanPath string, full bool) (string, error) {
	root := filepath.Clean(destination)
	if root == string(filepath.Separator) {
		return cleanPath, nil
	}

	rel, err := filepath.Rel(root, cleanPath)
	if err != nil {
		return "", err
	}

	if full || rel == "." {
		return safepath.ResolveInRoot(root, rel)
	}

	parent, err := safepath.ResolveInRoot(root, filepath.Dir(rel))
	if err != nil {
		return "", err
	}

	return filepath.Join(parent, filepath.Base(rel)), nil
}

// prepareTarget resolves the entry path, creates its parent directory inside the
// root and removes a pre-existing symlink at the final component.
func prepareTarget(destination, cleanPath string) (string, error) {
	p, err := resolveWithin(destination, cleanPath, false)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}

	if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(p)
	}

	return p, nil
}

// extractHardlink creates path as a hardlink to an already-extracted archive member.
func extractHardlink(destination, cleanPath, linkName string) error {
	srcClean, err := safeJoin(destination, linkName)
	if err != nil {
		return err
	}

	src, err := resolveWithin(destination, srcClean, false)
	if err != nil {
		return err
	}

	dst, err := prepareTarget(destination, cleanPath)
	if err != nil {
		return err
	}

	_ = os.Remove(dst)

	return os.Link(src, dst)
}
