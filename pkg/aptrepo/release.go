package aptrepo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Release holds the subset of a Release file needed to verify component indexes.
type Release struct {
	Codename string
	Suite    string
	// SHA256 maps filename → (hash, size)
	SHA256 map[string]hashEntry
}

type hashEntry struct {
	Hash string // hex SHA256
	Size int64
}

// fetchRelease downloads InRelease (clear-signed) or Release+Release.gpg,
// parses it, and returns the structured Release.
func fetchRelease(ctx context.Context, baseURL, suite string) (*Release, error) {
	// Try InRelease first (clear-signed).
	releaseURL := strings.TrimRight(baseURL, "/") + "/dists/" + suite + "/InRelease"

	data, err := httpFetch(ctx, releaseURL)
	if err == nil {
		return parseRelease(data)
	}

	// Fall back to Release (plain text).
	releaseURL = strings.TrimRight(baseURL, "/") + "/dists/" + suite + "/Release"

	data, err = httpFetch(ctx, releaseURL)
	if err != nil {
		return nil, err
	}

	return parseRelease(data)
}

// parseRelease handles both InRelease (clear-signed) and plain Release.
// For InRelease, strip the PGP signature blocks.
func parseRelease(data []byte) (*Release, error) {
	body := stripClearsignArmor(data)
	rel := &Release{SHA256: make(map[string]hashEntry)}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	inSHA256 := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Continuation lines start with whitespace.
		if line[0] == ' ' || line[0] == '\t' {
			if !inSHA256 {
				continue
			}

			// Format: " <hash>  <size>   <filename>"
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}

			size, _ := strconv.ParseInt(fields[1], 10, 64)
			rel.SHA256[fields[2]] = hashEntry{Hash: fields[0], Size: size}

			continue
		}

		inSHA256 = false

		if k, v, ok := strings.Cut(line, ":"); ok {
			v = strings.TrimSpace(v)

			switch k {
			case "Codename":
				rel.Codename = v
			case "Suite":
				rel.Suite = v
			case "SHA256":
				inSHA256 = true
			}
		}
	}

	return rel, scanner.Err()
}

// stripClearsignArmor extracts the body between "-----BEGIN PGP SIGNED MESSAGE-----"
// and "-----BEGIN PGP SIGNATURE-----". If the input has no armor, return it unchanged.
func stripClearsignArmor(data []byte) []byte {
	// Check if this is a clear-signed message.
	if !bytes.Contains(data, []byte("-----BEGIN PGP SIGNED MESSAGE-----")) {
		// Not armored, return as-is.
		return data
	}

	// Find the start of the signed message (after the armor headers).
	start := bytes.Index(data, []byte("-----BEGIN PGP SIGNED MESSAGE-----"))
	if start == -1 {
		return data
	}

	// Skip to the end of the armor header line.
	nlIdx := bytes.Index(data[start:], []byte("\n"))
	if nlIdx == -1 {
		return data
	}

	start = start + nlIdx + 1

	// Skip armor headers (Hash: SHA256, etc.) until we hit a blank line.
	for {
		eol := bytes.Index(data[start:], []byte("\n"))
		if eol == -1 {
			return data
		}

		line := data[start : start+eol]
		if len(line) == 0 || (len(line) == 1 && line[0] == '\r') {
			// Blank line marks end of headers.
			start = start + eol + 1
			break
		}

		start = start + eol + 1
	}

	// Find the signature block.
	sigStart := bytes.Index(data[start:], []byte("-----BEGIN PGP SIGNATURE-----"))
	if sigStart == -1 {
		// No signature block found, return everything from start.
		return data[start:]
	}

	// Return the body (between headers and signature).
	body := data[start : start+sigStart]

	// Trim trailing whitespace/newlines.
	return bytes.TrimRight(body, "\r\n \t")
}

// httpFetch downloads a URL and returns the raw bytes.
func httpFetch(ctx context.Context, fetchURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, fetchURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// encodeListFilename converts (baseURL, suite, relPath) into apt's list filename:
// e.g. ("https://archive.ubuntu.com/ubuntu/", "jammy", "main/binary-amd64/Packages.xz")
//
//	→ "archive.ubuntu.com_ubuntu_dists_jammy_main_binary-amd64_Packages.xz"
func encodeListFilename(baseURL, suite, relPath string) string {
	u, _ := url.Parse(baseURL)
	if u == nil {
		return ""
	}

	p := strings.TrimSuffix(u.Path, "/")
	prefix := u.Host + strings.ReplaceAll(p, "/", "_")
	full := prefix + "_dists_" + suite + "_" + strings.ReplaceAll(relPath, "/", "_")

	return full
}
