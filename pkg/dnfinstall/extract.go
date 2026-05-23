package dnfinstall

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/cavaliergopher/cpio"
	"github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// rpmEntry represents a parsed RPM file and its extracted file metadata.
// Used internally to track extraction state for later rpmdb write (Phase 3).
type rpmEntry struct {
	Path   string           // source rpm file path
	RPM    *rpmutils.Rpm    // parsed RPM header and payload reader
	Files  []installedFile  // metadata for each extracted file
}

// installedFile tracks metadata for a file extracted from an RPM.
// Used by Phase 3 (rpmdb write) to update the RPM database.
type installedFile struct {
	Path       string // absolute path on disk
	Mode       os.FileMode
	Size       int64
	SHA256     string // if computed
	IsDir      bool
	IsSymlink  bool
	IsConfig   bool
	IsNoReplace bool
	LinkTarget string
}

// extractRPM opens an .rpm at path and extracts its CPIO payload to rootDir.
// Returns the parsed *rpmutils.Rpm so callers can read header tags for
// scriptlets/GPG/rpmdb-write later. Caller owns lifetime of the file handle.
//
// This is the Phase 2 implementation of RPM extraction.
func extractRPM(ctx context.Context, path, rootDir string, opts Options) (*rpmEntry, error) {
	// Open the RPM file.
	f, err := os.Open(path) // #nosec G304 — path is validated by caller
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM file").
			WithOperation("extractRPM").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	// Parse the RPM header and payload.
	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to parse RPM").
			WithOperation("extractRPM").
			WithContext("path", path)
	}

	return extractRPMWithHeader(ctx, path, rootDir, rpm, opts)
}

// extractRPMWithHeader extracts an already-parsed RPM to rootDir.
// Used by installPackage to avoid re-parsing the RPM after scriptlets.
func extractRPMWithHeader(ctx context.Context, path, rootDir string, rpm *rpmutils.Rpm, opts Options) (*rpmEntry, error) {

	// Sanity check: confirm rootDir exists.
	if rootDir != "/" {
		if _, err := os.Stat(rootDir); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "rootDir does not exist").
				WithOperation("extractRPM").
				WithContext("rootDir", rootDir)
		}
	}

	// Get the decompressed CPIO payload reader.
	payloadReader, err := rpm.PayloadReaderExtended()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to get payload reader").
			WithOperation("extractRPM").
			WithContext("path", path)
	}

	// Wrap in CPIO reader.
	cpioReader := cpio.NewReader(payloadReader)

	// Track extracted files for later rpmdb write.
	var files []installedFile

	// Track hardlinks by (inode, device) for deferred linking.
	hardlinks := make(map[uint64][]pendingLink)

	// Extract each CPIO entry.
	for {
		// Check context cancellation.
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "context cancelled").
				WithOperation("extractRPM")
		}

		hdr, err := cpioReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to read CPIO header").
				WithOperation("extractRPM").
				WithContext("path", path)
		}

		// Compute target path via safe-path check.
		targetPath, err := safeRPMPath(rootDir, hdr.Name)
		if err != nil {
			logger.Warn("skipping unsafe path in RPM archive", "path", hdr.Name, "error", err)
			continue
		}

		// Create parent directories.
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directories").
				WithOperation("extractRPM").
				WithContext("path", targetPath)
		}

		// Extract based on file type.
		if err := extractCPIOEntry(cpioReader, hdr, targetPath, rootDir, hardlinks); err != nil {
			return nil, err
		}

		// Track file metadata for rpmdb write.
		files = append(files, installedFile{
			Path:      targetPath,
			Mode:      hdr.FileInfo().Mode(),
			Size:      hdr.Size,
			IsDir:     hdr.FileInfo().IsDir(),
			IsSymlink: hdr.FileInfo().Mode()&os.ModeSymlink != 0,
			LinkTarget: hdr.Linkname,
		})
	}

	// Replay queued hardlinks.
	for _, links := range hardlinks {
		for _, link := range links {
			if err := os.Link(link.Source, link.Target); err != nil {
				return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create hardlink").
					WithOperation("extractRPM").
					WithContext("source", link.Source).
					WithContext("target", link.Target)
			}
		}
	}

	logger.Debug("extracted RPM", "path", path, "files", len(files))

	return &rpmEntry{
		Path:  path,
		RPM:   rpm,
		Files: files,
	}, nil
}

// pendingLink tracks a hardlink to be created after the source file is written.
type pendingLink struct {
	Source string
	Target string
}

// extractCPIOEntry extracts a single CPIO entry to the filesystem.
// Handles regular files, directories, symlinks, and hardlinks with proper
// sanitization.
func extractCPIOEntry(cpioReader *cpio.Reader, hdr *cpio.Header, targetPath, rootDir string, hardlinks map[uint64][]pendingLink) error {
	const maxFileSize = 2 << 30 // 2 GiB

	// Determine inode key for hardlink tracking.
	inodeKey := uint64(hdr.Inode)

	switch {
	case hdr.FileInfo().IsDir():
		// Directory.
		if err := os.MkdirAll(targetPath, hdr.FileInfo().Mode()); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create directory").
				WithOperation("extractCPIOEntry").
				WithContext("path", targetPath)
		}

	case hdr.FileInfo().Mode()&os.ModeSymlink != 0:
		// Symlink.
		if err := safeRPMSymlinkTarget(rootDir, targetPath, hdr.Linkname); err != nil {
			logger.Warn("skipping unsafe RPM symlink",
				"path", hdr.Name, "target", hdr.Linkname, "error", err)
			return nil
		}

		if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
			// Ignore if already exists.
			if !os.IsExist(err) {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create symlink").
					WithOperation("extractCPIOEntry").
					WithContext("path", targetPath).
					WithContext("target", hdr.Linkname)
			}
		}

	case hdr.Links > 1:
		// Hardlink: queue for later linking after source is written.
		hardlinks[inodeKey] = append(hardlinks[inodeKey], pendingLink{
			Source: targetPath,
			Target: targetPath,
		})

	default:
		// Regular file.
		// Write to a sibling temp file then atomically rename over the target.
		// This avoids ETXTBSY ("text file busy") when overwriting a binary
		// that is currently being executed.
		tmpPath := targetPath + ".rpm-new"

		f, err := os.Create(tmpPath) // #nosec G304 — derived from safeRPMPath
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary file").
				WithOperation("extractCPIOEntry").
				WithContext("path", tmpPath)
		}

		if _, err := io.Copy(f, io.LimitReader(cpioReader, maxFileSize)); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to copy file contents").
				WithOperation("extractCPIOEntry").
				WithContext("path", tmpPath)
		}

		if err := f.Close(); err != nil {
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close file").
				WithOperation("extractCPIOEntry").
				WithContext("path", tmpPath)
		}

		// Preserve permissions before the rename.
		if err := os.Chmod(tmpPath, hdr.FileInfo().Mode()); err != nil {
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set file permissions").
				WithOperation("extractCPIOEntry").
				WithContext("path", tmpPath)
		}

		if err := os.Rename(tmpPath, targetPath); err != nil {
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to rename file").
				WithOperation("extractCPIOEntry").
				WithContext("from", tmpPath).
				WithContext("to", targetPath)
		}
	}

	return nil
}
