package dnfinstall

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"

	"github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// rpmEntry represents a parsed RPM file and its extracted file metadata.
type rpmEntry struct {
	Path  string
	RPM   *rpmutils.Rpm
	Files []installedFile
}

// installedFile tracks metadata for a file extracted from an RPM.
type installedFile struct {
	Path        string
	Mode        os.FileMode
	Size        int64
	SHA256      string
	IsDir       bool
	IsSymlink   bool
	IsConfig    bool
	IsNoReplace bool
	LinkTarget  string
}

// hardlinkTracker manages hardlink resolution during RPM extraction.
// CPIO payload ordering is arbitrary: any entry may carry the file content
// while others (IsLink) are zero-length hardlink references. This tracker
// ensures all hardlinks are resolved correctly regardless of entry order.
type hardlinkTracker struct {
	// data maps inode key to the path of the data-bearing entry.
	data map[uint64]string
	// placeholders maps inode key to paths of hardlink-only entries seen
	// before the data entry, so they can be linked once data is written.
	placeholders map[uint64][]string
}

// newHardlinkTracker creates a new hardlink tracker.
func newHardlinkTracker() *hardlinkTracker {
	return &hardlinkTracker{
		data:         make(map[uint64]string),
		placeholders: make(map[uint64][]string),
	}
}

// handleHardlink processes a hardlink entry. If the data-bearing entry has
// already been written, it creates the hardlink immediately. Otherwise, it
// queues the path for linking once the data is written.
func (ht *hardlinkTracker) handleHardlink(inodeKey uint64,
	targetPath string) error {
	if data, ok := ht.data[inodeKey]; ok {
		// Data already written — create hardlink now.
		return makeHardlink(data, targetPath)
	}

	// Data not yet seen — queue for later linking.
	ht.placeholders[inodeKey] = append(ht.placeholders[inodeKey], targetPath)

	return nil
}

// materializeData writes the data-bearing entry and links all queued
// placeholders to it.
func (ht *hardlinkTracker) materializeData(inodeKey uint64,
	targetPath string) error {
	ht.data[inodeKey] = targetPath

	// Link queued placeholders to the newly written data file.
	for _, placeholder := range ht.placeholders[inodeKey] {
		if err := makeHardlink(targetPath, placeholder); err != nil {
			return err
		}
	}

	return nil
}

// extractRPM opens an .rpm at path and extracts its CPIO payload to rootDir.
//
//nolint:unparam // opts kept for API symmetry with extractRPMWithHeader; current callers pass Options{}
func extractRPM(ctx context.Context, path, rootDir string, opts Options) (*rpmEntry, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM file").
			WithOperation("extractRPM").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to parse RPM").
			WithOperation("extractRPM").
			WithContext("path", path)
	}

	return extractRPMWithHeader(ctx, path, rootDir, rpm, opts)
}

// RPM file type bits (POSIX mode).
const (
	rpmTypeMask = 0o170000
	rpmTypeDir  = 0o040000
	rpmTypeLink = 0o120000
	rpmTypeReg  = 0o100000
)

// handleRegularEntry processes a regular file or hardlink entry.
// If pr.IsLink() is true, it handles hardlink resolution via the tracker.
// Otherwise, it writes the file data and materializes any queued hardlinks.
func handleRegularEntry(pr rpmutils.PayloadReader, targetPath string,
	perm os.FileMode, inodeKey uint64, tracker *hardlinkTracker) error {
	if pr.IsLink() {
		return tracker.handleHardlink(inodeKey, targetPath)
	}

	if err := writeRegularFile(pr, targetPath, perm); err != nil {
		return err
	}

	return tracker.materializeData(inodeKey, targetPath)
}

// extractRPMWithHeader extracts an already-parsed RPM to rootDir using
// rpmutils' own PayloadReader, which handles cpio framing internally.
//
//nolint:gocyclo,cyclop,nestif // RPM CPIO extraction has many distinct branches by entry type
func extractRPMWithHeader(ctx context.Context, path, rootDir string, rpm *rpmutils.Rpm, _ Options) (*rpmEntry, error) {
	if rootDir != "/" {
		if _, err := os.Stat(rootDir); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "rootDir does not exist").
				WithOperation("extractRPM").
				WithContext("rootDir", rootDir)
		}
	}

	pr, err := rpm.PayloadReaderExtended()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to get payload reader").
			WithOperation("extractRPM").
			WithContext("path", path)
	}

	var files []installedFile

	tracker := newHardlinkTracker()

	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "context cancelled").
				WithOperation("extractRPM")
		}

		fi, err := pr.Next()
		if stderrors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeValidation, "failed to read payload entry").
				WithOperation("extractRPM").
				WithContext("path", path)
		}

		name := fi.Name()
		if len(name) > 1 && name[0] == '.' && name[1] == '/' {
			name = name[1:]
		}

		targetPath, err := safeRPMPath(rootDir, name)
		if err != nil {
			logger.Warn("skipping unsafe path in RPM archive", "path", name, "error", err)
			continue
		}

		mode := uint32(fi.Mode()) //nolint:gosec
		fileType := mode & rpmTypeMask
		perm := os.FileMode(mode & 0o7777)

		inodeKey := (uint64(uint32(fi.Device())) << 32) | uint64(uint32(fi.Inode())) //nolint:gosec

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directories").
				WithOperation("extractRPM").
				WithContext("path", targetPath)
		}

		switch fileType {
		case rpmTypeDir:
			if err := os.MkdirAll(targetPath, perm); err != nil {
				return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create directory").
					WithOperation("extractRPM").
					WithContext("path", targetPath)
			}

		case rpmTypeLink:
			if err := safeRPMSymlinkTarget(rootDir, targetPath, fi.Linkname()); err != nil {
				logger.Warn("skipping unsafe RPM symlink",
					"path", name, "target", fi.Linkname(), "error", err)

				continue
			}

			_ = os.Remove(targetPath)
			if err := os.Symlink(fi.Linkname(), targetPath); err != nil {
				return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create symlink").
					WithOperation("extractRPM").
					WithContext("path", targetPath).
					WithContext("target", fi.Linkname())
			}

		case rpmTypeReg, 0:
			if err := handleRegularEntry(pr, targetPath, perm, inodeKey,
				tracker); err != nil {
				return nil, err
			}

		default:
			logger.Debug("skipping non-regular RPM entry", "path", name, "mode", mode)
			continue
		}

		files = append(files, installedFile{
			Path:       targetPath,
			Mode:       perm,
			Size:       fi.Size(),
			IsDir:      fileType == rpmTypeDir,
			IsSymlink:  fileType == rpmTypeLink,
			LinkTarget: fi.Linkname(),
		})
	}

	logger.Debug("extracted RPM", "path", path, "files", len(files))

	return &rpmEntry{
		Path:  path,
		RPM:   rpm,
		Files: files,
	}, nil
}

// writeRegularFile streams the current payload entry to targetPath atomically.
func writeRegularFile(pr rpmutils.PayloadReader, targetPath string, perm os.FileMode) error {
	const maxFileSize = 2 << 30 // 2 GiB

	tmpPath := targetPath + ".rpm-new"

	f, err := os.Create(tmpPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary file").
			WithOperation("extractCPIOEntry").
			WithContext("path", tmpPath)
	}

	if _, err := io.Copy(f, io.LimitReader(pr, maxFileSize)); err != nil {
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

	if err := os.Chmod(tmpPath, perm); err != nil {
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

	return nil
}

// makeHardlink creates target as a hardlink to source.
func makeHardlink(source, target string) error {
	if source == target {
		return nil
	}

	_ = os.Remove(target)
	if err := os.Link(source, target); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create hardlink").
			WithOperation("extractCPIOEntry").
			WithContext("source", source).
			WithContext("target", target)
	}

	return nil
}
