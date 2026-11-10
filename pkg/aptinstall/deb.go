// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall

import (
	"archive/tar"
	stderrors "errors"
	"io"
	"maps"
	"os"
	"strings"

	pkgdeb "github.com/M0Rf30/pkgformat/deb"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// debContents holds the parsed contents of a .deb's control.tar.
type debContents struct {
	// Control is the raw control file body (deb822 format).
	Control string
	// Md5sums is the md5sums file content (newline-separated).
	Md5sums string
	// Conffiles is the list of configuration files (newline-separated).
	Conffiles string
	// Scriptlets maps scriptlet names ("preinst", "postinst", etc.) to their bodies.
	Scriptlets map[string]string
	// Triggers is the triggers file content.
	Triggers string
	// Files is the list of file paths from data.tar (for .list file).
	Files []string
}

// openDEB opens and parses a .deb with the pkgformat reader. The returned
// close function releases the file; the DebFile stays valid until then.
func openDEB(debPath string) (*pkgdeb.DebFile, func(), error) {
	file, err := os.Open(debPath) //nolint:gosec
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeFileSystem, "open DEB").
			WithOperation("openDEB").WithContext("path", debPath)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return nil, nil, errors.Wrap(err, errors.ErrTypeFileSystem, "stat DEB").
			WithOperation("openDEB").WithContext("path", debPath)
	}

	deb, err := pkgdeb.ParseDebFileAt(file, info.Size())
	if err != nil {
		_ = file.Close()

		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, "parse DEB").
			WithOperation("openDEB").WithContext("path", debPath)
	}

	return deb, func() { _ = file.Close() }, nil
}

// parseDEB reads a .deb's control archive and lists its payload files.
// The data archive is NOT extracted; only the file list is returned.
func parseDEB(debPath string) (*debContents, error) {
	deb, closeFile, err := openDEB(debPath)
	if err != nil {
		return nil, err
	}

	defer closeFile()

	contents := &debContents{
		Control:    string(deb.Control["control"]),
		Md5sums:    string(deb.Control["md5sums"]),
		Conffiles:  string(deb.Control["conffiles"]),
		Triggers:   string(deb.Control["triggers"]),
		Scriptlets: make(map[string]string),
	}

	for _, name := range []string{"preinst", "postinst", "prerm", "postrm"} {
		if body, ok := deb.Control[name]; ok {
			contents.Scriptlets[name] = string(body)
		}
	}

	if contents.Control == "" {
		return nil, errors.New(errors.ErrTypeParser, "control file not found in DEB").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	data, err := deb.OpenData()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeParser, "open data archive").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	defer func() { _ = data.Close() }()

	contents.Files, err = listDataFiles(data)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeParser, "read data archive").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	return contents, nil
}

// listDataFiles returns the regular-file and link entries of a data tar
// stream, as dpkg lists them in <pkg>.list; directories and device nodes stay
// out.
func listDataFiles(r io.Reader) ([]string, error) {
	var files []string

	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if stderrors.Is(err, io.EOF) {
			return files, nil
		}

		if err != nil {
			return nil, err
		}

		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeSymlink, tar.TypeLink: //nolint:staticcheck
		default:
			continue
		}

		if path := strings.TrimPrefix(hdr.Name, "./"); path != "" {
			files = append(files, path)
		}
	}
}

// parseControl parses a deb822-format control file and returns a map of fields.
func parseControl(control string) map[string]string {
	fields := make(map[string]string)

	_ = deb822.Parse(strings.NewReader(control), func(stanzaMap deb822.Stanza) error {
		// Copy all fields from the stanza into the result map
		maps.Copy(fields, stanzaMap)

		return nil
	})

	return fields
}
