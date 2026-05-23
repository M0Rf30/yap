package aptinstall

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/m0rf30/ar"
	"github.com/ulikunitz/xz"
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
		return nil, fmt.Errorf("open DEB: %w", err)
	}

	defer func() { _ = file.Close() }()

	arReader, err := ar.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("parse AR archive: %w", err)
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

			return nil, fmt.Errorf("read AR header: %w", err)
		}

		switch {
		case strings.HasPrefix(header.Name, "control.tar"):
			if err := parseControlTar(arReader, header.Name, contents); err != nil {
				return nil, fmt.Errorf("parse control.tar: %w", err)
			}

		case strings.HasPrefix(header.Name, "data.tar"):
			if err := parseDataTar(arReader, header.Name, contents); err != nil {
				return nil, fmt.Errorf("parse data.tar: %w", err)
			}
		}
	}

	if contents.Control == "" {
		return nil, fmt.Errorf("control file not found in DEB")
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

			return fmt.Errorf("read tar entry: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(tr, maxControlEntry))
		if err != nil {
			return fmt.Errorf("read tar file: %w", err)
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

			return fmt.Errorf("read tar entry: %w", err)
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
		return nil, fmt.Errorf("peek magic: %w", err)
	}

	switch {
	case bytes.HasPrefix(head, magicGzip):
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}

		return gz, nil

	case bytes.HasPrefix(head, magicXz):
		xzr, err := xz.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("xz reader: %w", err)
		}
		// xz.Reader doesn't implement Close.
		return io.NopCloser(xzr), nil

	case bytes.HasPrefix(head, magicZstd):
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("zstd reader: %w", err)
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

	scanner := bufio.NewScanner(strings.NewReader(control))

	var (
		currentField string
		currentValue strings.Builder
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Continuation line (starts with space or tab).
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			currentValue.WriteString("\n")
			currentValue.WriteString(strings.TrimPrefix(line, " "))

			continue
		}

		// Flush previous field.
		if currentField != "" {
			fields[currentField] = currentValue.String()
		}

		// Parse new field line.
		if field, value, ok := strings.Cut(line, ":"); ok {
			currentField = field

			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(value))
		} else {
			currentField = ""
		}
	}

	// Flush last field.
	if currentField != "" {
		fields[currentField] = currentValue.String()
	}

	return fields
}
