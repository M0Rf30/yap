// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package archive

import (
	"os"

	pkgdeb "github.com/M0Rf30/pkgformat/deb"
	pkgrpm "github.com/M0Rf30/pkgformat/rpm"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// ExtractDEB unpacks the data archive of a Debian package (.deb) into destDir.
// The extraction is confined to destDir by the pkgformat reader: entries that
// would land outside it, by name or through a link, are refused.
func ExtractDEB(packagePath, destDir string) error {
	file, err := os.Open(packagePath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open DEB package").
			WithContext("path", packagePath).
			WithOperation("ExtractDEB")
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to stat DEB package").
			WithContext("path", packagePath).
			WithOperation("ExtractDEB")
	}

	deb, err := pkgdeb.ParseDebFileAt(file, info.Size())
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to read DEB package").
			WithContext("path", packagePath).
			WithOperation("ExtractDEB")
	}

	if err := deb.Extract(destDir, pkgdeb.ExtractOptions{}); err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to extract DEB package").
			WithContext("path", packagePath).
			WithOperation("ExtractDEB")
	}

	return nil
}

// ExtractRPM unpacks the payload of an RPM package (cpio.{gz,xz,zst,...}) into
// destDir. Absolute entry names ("/opt/...", as yap's own packages carry) are
// rooted in destDir, and the extraction is confined to it by the pkgformat
// reader.
func ExtractRPM(packagePath, destDir string) error {
	file, err := os.Open(packagePath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM package").
			WithContext("path", packagePath).
			WithOperation("ExtractRPM")
	}

	defer func() { _ = file.Close() }()

	rpm, err := pkgrpm.ReadRpm(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to read RPM header").
			WithContext("path", packagePath).
			WithOperation("ExtractRPM")
	}

	// Same bound the DEB extraction applies, against decompression bombs.
	rpm.SetMaxPayloadSize(pkgdeb.DefaultMaxExtractBytes)

	if err := rpm.ExpandPayload(destDir); err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to extract RPM payload").
			WithContext("path", packagePath).
			WithOperation("ExtractRPM")
	}

	return nil
}
