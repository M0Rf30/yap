//go:build linux

// Package rootless implements a daemon-free container runtime for YAP using
// go-containerregistry for image pulls and rootlesskit for isolated execution.
package rootless

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
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

// PullImage pulls the YAP builder image for distro from the registry using
// pure Go (no CLI required) and extracts it to a local rootfs directory.
func PullImage(distro string) error {
	ref := constants.DockerOrg + distro
	logger.Info("pulling image", "ref", ref)

	img, err := crane.Pull(ref)
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

	logger.Info("saving OCI image layout", "path", storePath)

	if err := crane.SaveOCI(img, storePath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to save OCI image layout").
			WithOperation("PullImage").
			WithContext("path", storePath)
	}

	logger.Info("extracting rootfs", "distro", distro)

	return extractRootfs(img, distro)
}

// extractRootfs flattens all image layers into a rootfs directory.
// Existing rootfs is removed and recreated to ensure a clean state.
func extractRootfs(img v1.Image, distro string) error {
	rootfs, err := rootfsPath(distro)
	if err != nil {
		return err
	}

	// Remove stale rootfs before re-extracting.
	if err := os.RemoveAll(rootfs); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to remove stale rootfs").
			WithOperation("extractRootfs").
			WithContext("path", rootfs)
	}

	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create rootfs directory").
			WithOperation("extractRootfs").
			WithContext("path", rootfs)
	}

	// crane.Export flattens all layers into a single tar stream.
	pr, pw := io.Pipe()

	exportErr := make(chan error, 1)

	go func() {
		exportErr <- crane.Export(img, pw)

		if err := pw.Close(); err != nil {
			logger.Warn("pipe writer close error", "error", err)
		}
	}()

	if err := extractTar(pr, rootfs); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to extract rootfs tar").
			WithOperation("extractRootfs").
			WithContext("distro", distro)
	}

	if err := <-exportErr; err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to export image layers").
			WithOperation("extractRootfs").
			WithContext("distro", distro)
	}

	logger.Info("rootfs ready", "path", rootfs)

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
		if err == io.EOF {
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

		// Sanitize path to prevent traversal.
		target := filepath.Join(destDir, filepath.Clean("/"+hdr.Name)) //nolint:gosec

		if err := extractTarEntry(tr, hdr, target, destDir); err != nil {
			return err
		}
	}

	return nil
}

// extractTarEntry handles a single tar entry.
func extractTarEntry(tr *tar.Reader, hdr *tar.Header, target, destDir string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(hdr.Mode)) //nolint:gosec

	case tar.TypeReg:
		return extractRegularFile(tr, hdr, target)

	case tar.TypeSymlink:
		return extractSymlink(hdr, target)

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

func extractSymlink(hdr *tar.Header, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directory").
			WithOperation("extractSymlink").
			WithContext("path", target)
	}

	_ = os.Remove(target)

	return os.Symlink(hdr.Linkname, target)
}

func extractHardLink(hdr *tar.Header, target, destDir string) error {
	linkTarget := filepath.Join(destDir, filepath.Clean("/"+hdr.Linkname)) //nolint:gosec

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
