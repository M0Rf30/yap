package aptinstall

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"io"
	"os"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
	"github.com/klauspost/compress/zstd"
	"github.com/m0rf30/ar"
	"github.com/ulikunitz/xz"

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

// parseDEB opens a .deb, extracts control.tar and data.tar, and returns metadata.
// The data.tar is NOT extracted to /; only the file list is returned.
func parseDEB(debPath string) (*debContents, error) {
	file, err := os.Open(debPath) // #nosec G304 - debPath is from trusted apt index metadata
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "open DEB").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	defer func() { _ = file.Close() }()

	arReader, err := ar.NewReader(file)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeParser, "parse AR archive").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	contents := &debContents{
		Scriptlets: make(map[string]string),
	}

	for {
		header, err := arReader.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return nil, errors.Wrap(err, errors.ErrTypeParser, "read AR header").
				WithOperation("parseDEB")
		}

		switch {
		case strings.HasPrefix(header.Name, "control.tar"):
			if err := parseControlTar(arReader, header.Name, contents); err != nil {
				return nil, errors.Wrap(err, errors.ErrTypeParser, "parse control.tar").
					WithOperation("parseDEB")
			}

		case strings.HasPrefix(header.Name, "data.tar"):
			if err := parseDataTar(arReader, header.Name, contents); err != nil {
				return nil, errors.Wrap(err, errors.ErrTypeParser, "parse data.tar").
					WithOperation("parseDEB")
			}
		}
	}

	if contents.Control == "" {
		return nil, errors.New(errors.ErrTypeParser, "control file not found in DEB").
			WithOperation("parseDEB").WithContext("path", debPath)
	}

	return contents, nil
}

// parseControlTar extracts and parses the control.tar member of a .deb.
//
// Each entry body is read via an io.LimitReader so a malformed .deb cannot
// OOM the process by claiming a multi-gigabyte control file. Real control
// files are tiny (kilobytes); the 64 MiB cap is a generous safety net.
func parseControlTar(r io.Reader, name string, contents *debContents) error {
	const maxControlEntry = 64 << 20

	decompressed, err := decompressStream(r, name)
	if err != nil {
		return err
	}

	defer func() { _ = decompressed.Close() }()

	tr := tar.NewReader(decompressed)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return errors.Wrap(err, errors.ErrTypeParser, "read tar entry").
				WithOperation("parseControlTar")
		}

		body, err := io.ReadAll(io.LimitReader(tr, maxControlEntry))
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "read tar file").
				WithOperation("parseControlTar")
		}

		// Strip leading "./" from tar entry names.
		name := strings.TrimPrefix(hdr.Name, "./")

		switch name {
		case "control":
			contents.Control = string(body)
		case "md5sums":
			contents.Md5sums = string(body)
		case "conffiles":
			contents.Conffiles = string(body)
		case "triggers":
			contents.Triggers = string(body)
		case "preinst", "postinst", "prerm", "postrm":
			contents.Scriptlets[name] = string(body)
		}
	}

	return nil
}

// parseDataTar extracts the file list from the data.tar member of a .deb.
func parseDataTar(r io.Reader, name string, contents *debContents) error {
	decompressed, err := decompressStream(r, name)
	if err != nil {
		return err
	}

	defer func() { _ = decompressed.Close() }()

	tr := tar.NewReader(decompressed)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return errors.Wrap(err, errors.ErrTypeParser, "read tar entry").
				WithOperation("parseDataTar")
		}

		// Skip directories and other non-regular files.
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA { //nolint:staticcheck
			continue
		}

		// Strip leading "./" from tar entry names.
		path := strings.TrimPrefix(hdr.Name, "./")
		if path != "" {
			contents.Files = append(contents.Files, path)
		}
	}

	return nil
}

// Magic-byte prefixes used to identify compression formats. Sniffing the
// first few bytes is more reliable than trusting a filename suffix —
// extractDataTar buffers the data.tar member into an os.CreateTemp file
// whose pattern substitution strips the real ".gz"/".xz"/".zst"
// extension, so a name-based switch would always fall through to the
// "uncompressed" default and feed compressed bytes into archive/tar.
var (
	magicGzip  = []byte{0x1F, 0x8B}
	magicXz    = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	magicZstd  = []byte{0x28, 0xB5, 0x2F, 0xFD}
	magicBzip2 = []byte{'B', 'Z', 'h'}
)

// decompressStream returns a decompressed reader for a tar payload.
// Detection is by magic bytes, not by the supplied `name` (which the
// caller may not control — see comment on the magic-prefix vars).
// The caller must close the returned reader.
func decompressStream(r io.Reader, _ string) (io.ReadCloser, error) {
	// bufio so we can Peek the magic without consuming it.
	br := bufio.NewReader(r)

	head, err := br.Peek(6)
	if err != nil && err != io.EOF { //nolint:errorlint // io.EOF sentinel
		return nil, errors.Wrap(err, errors.ErrTypeParser, "peek magic").
			WithOperation("decompressStream")
	}

	switch {
	case bytes.HasPrefix(head, magicGzip):
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeParser, "gzip reader").
				WithOperation("decompressStream")
		}

		return gz, nil

	case bytes.HasPrefix(head, magicXz):
		xzr, err := xz.NewReader(br)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeParser, "xz reader").
				WithOperation("decompressStream")
		}
		// xz.Reader doesn't implement Close.
		return io.NopCloser(xzr), nil

	case bytes.HasPrefix(head, magicZstd):
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeParser, "zstd reader").
				WithOperation("decompressStream")
		}

		return zr.IOReadCloser(), nil

	case bytes.HasPrefix(head, magicBzip2):
		return io.NopCloser(bzip2.NewReader(br)), nil

	default:
		// No recognised magic → assume already-uncompressed tar.
		return io.NopCloser(br), nil
	}
}

// parseControl parses a deb822-format control file and returns a map of fields.
func parseControl(control string) map[string]string {
	fields := make(map[string]string)

	_ = deb822.Parse(strings.NewReader(control), func(stanzaMap deb822.Stanza) error {
		// Copy all fields from the stanza into the result map
		for k, v := range stanzaMap {
			fields[k] = v
		}
		return nil
	})

	return fields
}
