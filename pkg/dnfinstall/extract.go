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

// extractRPM opens an .rpm at path and extracts its CPIO payload to rootDir.
func extractRPM(ctx context.Context, path, rootDir string, opts Options) (*rpmEntry, error) {
	f, err := os.Open(path) // #nosec G304 — path validated by caller
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

// extractRPMWithHeader extracts an already-parsed RPM to rootDir using
// rpmutils' own PayloadReader, which handles cpio framing internally.
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

	hardlinkSources := make(map[uint64]string)

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

		mode := uint32(fi.Mode()) // #nosec G115
		fileType := mode & rpmTypeMask
		perm := os.FileMode(mode & 0o7777)

		// #nosec G115 — RPM device/inode constrained by file format
		inodeKey := (uint64(uint32(fi.Device())) << 32) | uint64(uint32(fi.Inode()))

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
			if pr.IsLink() {
				// Hardlink with no contents — record placeholder; the last
				// file in the link group carries the bytes.
				if src, ok := hardlinkSources[inodeKey]; ok {
					if err := makeHardlink(src, targetPath); err != nil {
						return nil, err
					}
				} else {
					hardlinkSources[inodeKey] = targetPath
				}
			} else {
				if err := writeRegularFile(pr, targetPath, perm); err != nil {
					return nil, err
				}

				if placeholder, ok := hardlinkSources[inodeKey]; ok && placeholder != targetPath {
					if err := makeHardlink(targetPath, placeholder); err != nil {
						return nil, err
					}
				}

				hardlinkSources[inodeKey] = targetPath
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

	f, err := os.Create(tmpPath) // #nosec G304 — derived from safeRPMPath
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
