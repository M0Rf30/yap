//go:build linux

// Package rootless implements a daemon-free container runtime for YAP using
// go-containerregistry for image pulls and rootlesskit for isolated execution.
package rootless

import (
	"archive/tar"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

// imageStorePath returns the local OCI store path for a given distro image.
// Images are stored under ~/.local/share/yap/images/<distro>/.
func imageStorePath(distro string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to resolve home directory").
			WithOperation("imageStorePath")
	}

	return filepath.Join(home, ".local", "share", "yap", "images", distro), nil
}

// rootfsPath returns the path where the image rootfs is extracted for a distro.
func rootfsPath(distro string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to resolve home directory").
			WithOperation("rootfsPath")
	}

	return filepath.Join(home, ".local", "share", "yap", "rootfs", distro), nil
}

// PullImage pulls the YAP builder image for distro from the registry
// (no CLI required) and extracts it to a local rootfs directory.
func PullImage(distro string) error {
	ref := constants.DockerOrg + distro
	logger.Info(i18n.T("logger.rootless.info.pulling_image"), "ref", ref)

	img, err := crane.Pull(ref, crane.WithPlatform(&v1.Platform{
		OS:           "linux",
		Architecture: runtime.GOARCH,
	}))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork,
			fmt.Sprintf("failed to pull image %s", ref)).
			WithOperation("PullImage").
			WithContext("distro", distro)
	}

	storePath, err := imageStorePath(distro)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create image store directory").
			WithOperation("PullImage").
			WithContext("path", storePath)
	}

	logger.Info(i18n.T("logger.rootless.info.saving_oci_image_layout"), "path", storePath)

	if err := crane.SaveOCI(img, storePath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to save OCI image layout").
			WithOperation("PullImage").
			WithContext("path", storePath)
	}

	logger.Info(i18n.T("logger.rootless.info.extracting_rootfs"), "distro", distro)

	return extractRootfs(img, distro)
}

// resolveEntry maps an archive entry name to a host path inside destDir. The
// parent directory is resolved with chroot semantics through symlinks that
// already exist on disk (see safepath.ResolveInRoot); the final component is
// left unresolved so entries replace, rather than follow, an existing link.
func resolveEntry(destDir, name string) (string, error) {
	rel := filepath.Clean("/" + name)
	if rel == "/" {
		return destDir, nil
	}

	parent, err := safepath.ResolveInRoot(destDir, filepath.Dir(rel))
	if err != nil {
		return "", err //nolint:wrapcheck // wrapped by callers with entry context
	}

	return filepath.Join(parent, filepath.Base(rel)), nil
}

// extractRootfs flattens all image layers into a rootfs directory.
// The image is extracted into a sibling temporary directory and swapped into
// place only once extraction succeeded, so a failed pull never leaves a
// half-populated rootfs at the path RunInRootless treats as valid.
func extractRootfs(img v1.Image, distro string) error {
	rootfs, err := rootfsPath(distro)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(rootfs), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create rootfs parent directory").
			WithOperation("extractRootfs").
			WithContext("path", filepath.Dir(rootfs))
	}

	tmp, err := os.MkdirTemp(filepath.Dir(rootfs), "."+distro+".tmp-")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create temporary rootfs directory").
			WithOperation("extractRootfs").
			WithContext("path", rootfs)
	}

	if err := exportToDir(img, tmp); err != nil {
		_ = os.RemoveAll(tmp)

		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to extract rootfs").
			WithOperation("extractRootfs").
			WithContext("distro", distro)
	}

	if err := swapRootfs(tmp, rootfs); err != nil {
		_ = os.RemoveAll(tmp)

		return err
	}

	logger.Info(i18n.T("logger.rootless.info.rootfs_ready"), "path", rootfs)

	return nil
}

// exportToDir flattens img into destDir, propagating failures in either the
// export goroutine or the tar extraction to the other side so neither leaks.
func exportToDir(img v1.Image, destDir string) error {
	// crane.Export flattens all layers into a single tar stream.
	pr, pw := io.Pipe()

	exportErr := make(chan error, 1)

	go func() {
		err := crane.Export(img, pw)

		_ = pw.CloseWithError(err) // nil err behaves like Close (io.EOF)

		exportErr <- err
	}()

	if err := extractTar(pr, destDir); err != nil {
		// Unblock the export goroutine's pending write, then wait for it.
		_ = pr.CloseWithError(err)

		<-exportErr

		return err
	}

	if err := <-exportErr; err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to export image layers").
			WithOperation("exportToDir")
	}

	return nil
}

// swapRootfs moves the freshly extracted tree at newDir to rootfs, replacing
// any existing rootfs. The previous rootfs is restored if the swap fails.
func swapRootfs(newDir, rootfs string) error {
	backup := ""

	if _, err := os.Lstat(rootfs); err == nil {
		backup = newDir + ".old"

		if err := os.Rename(rootfs, backup); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem,
				"failed to move stale rootfs aside").
				WithOperation("swapRootfs").
				WithContext("path", rootfs)
		}
	}

	if err := os.Rename(newDir, rootfs); err != nil {
		if backup != "" {
			_ = os.Rename(backup, rootfs)
		}

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to install new rootfs").
			WithOperation("swapRootfs").
			WithContext("path", rootfs)
	}

	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			logger.Warn(i18n.T("logger.rootless.warn.failed_remove_rootlesskit_state"),
				"path", backup, "error", err)
		}
	}

	return nil
}

// extractTar writes the contents of a tar stream into destDir.
// Handles regular files, directories, symlinks, and hard links.
// Skips whiteout files (overlay deletion markers).
//
//nolint:gocyclo // inherent complexity of tar extraction with multiple entry types
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if stderrors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "failed to read tar entry").
				WithOperation("extractTarStream")
		}

		// Skip overlay whiteout files.
		base := filepath.Base(hdr.Name)
		if base == ".wh..wh..opq" || (len(base) > 4 && base[:4] == ".wh.") {
			continue
		}

		// Sanitize path to prevent traversal (zip-slip), then resolve the
		// parent through symlinks planted by earlier entries so later
		// writes cannot be redirected outside destDir.
		if _, err := safepath.Join(destDir, hdr.Name); err != nil {
			return errors.Wrap(err, errors.ErrTypeValidation, "unsafe tar entry path").
				WithOperation("extractTar").
				WithContext("entry", hdr.Name)
		}

		target, err := resolveEntry(destDir, hdr.Name)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeValidation, "unsafe tar entry path").
				WithOperation("extractTar").
				WithContext("entry", hdr.Name)
		}

		if err := extractTarEntry(tr, hdr, target, destDir); err != nil {
			return err
		}
	}

	return nil
}

// extractDir creates a directory entry. The full path is resolved through
// existing symlinks (chroot semantics) so an entry such as "var/run/" where
// run -> /run lands inside destDir instead of creating host directories.
func extractDir(hdr *tar.Header, destDir string) error {
	dir, err := safepath.ResolveInRoot(destDir, hdr.Name)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "unsafe directory in layer").
			WithOperation("extractDir").
			WithContext("entry", hdr.Name)
	}

	return os.MkdirAll(dir, os.FileMode(hdr.Mode)) //nolint:gosec
}

// extractTarEntry handles a single tar entry.
func extractTarEntry(tr *tar.Reader, hdr *tar.Header, target, destDir string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return extractDir(hdr, destDir)

	case tar.TypeReg:
		return extractRegularFile(tr, hdr, target)

	case tar.TypeSymlink:
		return extractSymlink(hdr, target, destDir)

	case tar.TypeLink:
		return extractHardLink(hdr, target, destDir)
	}

	return nil
}

func extractRegularFile(tr *tar.Reader, hdr *tar.Header, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directory").
			WithOperation("extractRegularFile").
			WithContext("path", target)
	}

	// A layer replacing a symlink with a regular file must replace the link,
	// not write through it (O_TRUNC follows symlinks).
	if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(target)
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create file").
			WithOperation("extractRegularFile").
			WithContext("path", target)
	}

	if _, err := io.Copy(f, tr); err != nil { //nolint:gosec
		_ = f.Close()

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write file").
			WithOperation("extractRegularFile").
			WithContext("path", target)
	}

	return f.Close()
}

// extractSymlink creates a symlink after validating that a relative target
// cannot escape destDir (a malicious layer could otherwise plant a link
// pointing outside the rootfs and write through it with a later entry).
func extractSymlink(hdr *tar.Header, target, destDir string) error {
	if err := safepath.SymlinkTarget(destDir, target, hdr.Linkname); err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "unsafe symlink in layer").
			WithOperation("extractSymlink").
			WithContext("path", target).
			WithContext("link_target", hdr.Linkname)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directory").
			WithOperation("extractSymlink").
			WithContext("path", target)
	}

	_ = os.Remove(target)

	return os.Symlink(hdr.Linkname, target)
}

func extractHardLink(hdr *tar.Header, target, destDir string) error {
	// Resolve the link source through already-extracted symlinks with chroot
	// semantics so it cannot point outside destDir; the final component is
	// kept as-is so a hard link to a symlink links the symlink itself.
	linkTarget, err := resolveEntry(destDir, hdr.Linkname)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "unsafe hard link in layer").
			WithOperation("extractHardLink").
			WithContext("link_target", hdr.Linkname)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directory").
			WithOperation("extractHardLink").
			WithContext("path", target)
	}

	_ = os.Remove(target)

	return os.Link(linkTarget, target)
}

// RootfsExists returns true if a rootfs has already been extracted for distro.
func RootfsExists(distro string) (bool, error) {
	rootfs, err := rootfsPath(distro)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(rootfs)
	if os.IsNotExist(err) {
		return false, nil
	}

	return err == nil, err
}

// RootfsDir returns the path to the extracted rootfs for distro.
func RootfsDir(distro string) (string, error) {
	return rootfsPath(distro)
}
