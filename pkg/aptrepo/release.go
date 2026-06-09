package aptrepo

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
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

// fetchRelease downloads InRelease (clear-signed) or Release+Release.gpg
// from baseURL/dists/suite/, verifies the OpenPGP signature against the
// supplied keyring (when verification is enabled), and returns the
// parsed hash manifest.
//
// allowUnverified controls the fallback policy when:
//   - the source declares no Signed-By and the default apt trust paths
//     are empty (no keyring at all); or
//   - the mirror serves no signature at all (plain Release without a
//     Release.gpg).
//
// A signature that exists and *fails to verify* is always fatal,
// regardless of allowUnverified — a forged signature is strictly worse
// than no signature, and silently accepting it would defeat the purpose
// of the entire verification subsystem.
func fetchRelease(
	ctx context.Context,
	baseURL, suite, signedBy string,
	allowUnverified bool,
) (*Release, error) {
	keyring, keyringErr := loadKeyringForSource(signedBy)

	// Try InRelease first (clear-signed) — the modern format.
	releaseURL := strings.TrimRight(baseURL, "/") + "/dists/" + suite + "/InRelease"

	if data, err := httpFetch(ctx, releaseURL); err == nil {
		body, verr := verifyInReleaseOrFallback(data, keyring, keyringErr, allowUnverified, baseURL)
		if verr != nil {
			return nil, verr
		}

		return parseReleaseBody(body)
	}

	// Fall back to Release + Release.gpg (legacy format still used by
	// many mirrors and most third-party repos).
	body, err := httpFetch(ctx,
		strings.TrimRight(baseURL, "/")+"/dists/"+suite+"/Release")
	if err != nil {
		return nil, err
	}

	sig, sigErr := httpFetch(ctx,
		strings.TrimRight(baseURL, "/")+"/dists/"+suite+"/Release.gpg")

	body, err = verifyDetachedOrFallback(body, sig, sigErr,
		keyring, keyringErr, allowUnverified, baseURL)
	if err != nil {
		return nil, err
	}

	return parseReleaseBody(body)
}

// verifyInReleaseOrFallback resolves the trust decision for a fetched
// InRelease document. Returns the unsigned body bytes on success.
func verifyInReleaseOrFallback(
	data []byte,
	keyring openpgp.EntityList,
	keyringErr error,
	allowUnverified bool,
	baseURL string,
) ([]byte, error) {
	if len(keyring) == 0 {
		// No trust anchor available. Either the source asked for one
		// (Signed-By was set but resolution failed) or the default
		// trust paths are empty.
		if !allowUnverified {
			return nil, keyringErr
		}

		logger.Warn(i18n.T("logger.aptrepo.warn.skipping_inrelease_signature_check"), "url", baseURL, "reason", keyringErr)

		return stripClearsignArmor(data), nil
	}

	res, err := verifyInRelease(data, keyring)
	if err == nil {
		logger.Info(i18n.T("logger.aptrepo.info.verified_inrelease_signature"), "url", baseURL, "signer", res.signer)

		return res.body, nil
	}

	// Unknown signer: the repo has a signature but none of our trusted keys
	// match. Treat this the same as "no trust anchor" — bypassable via opt-in.
	if errors.Is(err, ErrUnknownSigner) {
		if !allowUnverified {
			return nil, yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "InRelease signature verification failed").
				WithOperation("verifyInReleaseOrFallback").
				WithContext("url", baseURL)
		}

		logger.Warn(i18n.T("logger.aptrepo.warn.skipping_inrelease_signature_check_unknown"), "url", baseURL, "reason", err)

		return stripClearsignArmor(data), nil
	}

	// A bad signature (corrupted data, wrong key material) is *never* tolerated.
	if !errors.Is(err, ErrUnsigned) {
		return nil, yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "InRelease signature verification failed").
			WithOperation("verifyInReleaseOrFallback").
			WithContext("url", baseURL)
	}

	// File is not signed at all → defer to the opt-in.
	if !allowUnverified {
		return nil, yaperrors.New(yaperrors.ErrTypeValidation, "InRelease is not signed").
			WithOperation("verifyInReleaseOrFallback").
			WithContext("url", baseURL)
	}

	logger.Warn(i18n.T("logger.aptrepo.warn.accepting_unsigned_inrelease_opt"), "url", baseURL)

	return stripClearsignArmor(data), nil
}

// verifyDetachedOrFallback resolves the trust decision for a Release +
// Release.gpg pair.
func verifyDetachedOrFallback(
	body, sig []byte,
	sigErr error,
	keyring openpgp.EntityList,
	keyringErr error,
	allowUnverified bool,
	baseURL string,
) ([]byte, error) {
	// No Release.gpg at all → defer to the opt-in. Many third-party
	// "deb http://… ./" one-liners ship this way historically.
	if sigErr != nil {
		if !allowUnverified {
			return nil, yaperrors.Wrap(sigErr, yaperrors.ErrTypeNetwork, "Release.gpg not available").
				WithOperation("verifyDetachedOrFallback").
				WithContext("url", baseURL)
		}

		logger.Warn(i18n.T("logger.aptrepo.warn.accepting_unsigned_release_opt"), "url", baseURL, "reason", sigErr)

		return body, nil
	}

	if len(keyring) == 0 {
		if !allowUnverified {
			return nil, keyringErr
		}

		logger.Warn(i18n.T("logger.aptrepo.warn.skipping_release_gpg_signature"), "url", baseURL, "reason", keyringErr)

		return body, nil
	}

	res, err := verifyDetachedRelease(body, sig, keyring)
	if err == nil {
		logger.Info(i18n.T("logger.aptrepo.info.verified_release_gpg_signature"), "url", baseURL, "signer", res.signer)

		return res.body, nil
	}

	// Unknown signer: bypassable via opt-in (same policy as InRelease).
	if errors.Is(err, ErrUnknownSigner) {
		if !allowUnverified {
			return nil, yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "Release.gpg signature verification failed").
				WithOperation("verifyDetachedOrFallback").
				WithContext("url", baseURL)
		}

		logger.Warn(i18n.T("logger.aptrepo.warn.skipping_release_gpg_signature_check"), "url", baseURL, "reason", err)

		return body, nil
	}

	return nil, yaperrors.Wrap(err, yaperrors.ErrTypeValidation, "Release.gpg signature verification failed").
		WithOperation("verifyDetachedOrFallback").
		WithContext("url", baseURL)
}

// parseReleaseBody parses the (already-verified, signature-stripped)
// hash manifest from a Release / InRelease document into the structured
// form used to validate component indexes.
func parseReleaseBody(body []byte) (*Release, error) {
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
//
// SECURITY: This function only strips the armor. It does NOT verify the
// signature. Callers that need verification should use verifyInRelease
// instead. The strip-only path remains for the opt-in AllowUnverifiedRepos
// fallback and for compatibility tests.
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

// maxReleaseBytes caps an InRelease/Release response at 16 MiB. Real
// release files are well under 1 MiB; the cap defends against an unbounded
// stream from a malicious or buggy mirror.
const maxReleaseBytes = 16 << 20

// httpFetch downloads a URL and returns the raw bytes, capped at
// maxReleaseBytes. Transient network failures are retried per the
// httpclient retry policy.
func httpFetch(ctx context.Context, fetchURL string) ([]byte, error) {
	var data []byte

	err := httpclient.WithRetry(ctx, fetchURL, func() error {
		var err error

		data, err = httpFetchOnce(ctx, fetchURL)

		return err
	})

	return data, err
}

// httpFetchOnce performs a single fetch attempt.
func httpFetchOnce(ctx context.Context, fetchURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if err := httpclient.CheckStatus(resp, fetchURL); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(httpclient.LimitedBodyN(resp, maxReleaseBytes))
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
