// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Sentinel errors, matchable with errors.Is.
var (
	// ErrNoRepository means no repository is configured for the artifact's format.
	ErrNoRepository = goerrors.New("no repository configured for package format")
	// ErrConflict means the repository already holds a package with the same
	// identity but different bytes. Hosted deb/rpm packages are immutable.
	ErrConflict = goerrors.New("package already exists with different content")
	// ErrChecksumMismatch means the server stored or reports other bytes than
	// the ones that were sent.
	ErrChecksumMismatch = goerrors.New("checksum mismatch between local file and server")
)

// Status is the outcome of publishing one file.
type Status string

// Possible publish outcomes.
const (
	// StatusUploaded: the server accepted the file and reported the expected SHA-256.
	StatusUploaded Status = "uploaded"
	// StatusUnchanged: the repository already held identical bytes; nothing was sent
	// again (or the server answered 409 and the stored bytes matched).
	StatusUnchanged Status = "unchanged"
)

// Result describes one published file (a package or a signature sidecar).
type Result struct {
	// File is the local path.
	File string
	// Format is the detected package format (empty for sidecars of raw formats).
	Format string
	// Repo is the OpenArtifact repository.
	Repo string
	// Remote is the server-side location: the raw path, or the upload endpoint.
	Remote string
	// SHA256 is the lowercase hex digest of the local file.
	SHA256 string
	// Size is the file size in bytes.
	Size int64
	// Status is the outcome.
	Status Status
}

const (
	maxErrorBody   = 4096
	listPageSize   = 100
	maxListPages   = 1000
	rawContentType = "application/octet-stream"
)

// Publisher uploads packages to one OpenArtifact server.
type Publisher struct {
	cfg    Config
	base   *url.URL
	client *http.Client
}

// New validates cfg and returns a Publisher.
func New(c *Config) (*Publisher, error) {
	cfg := *c

	base, err := cfg.validate()
	if err != nil {
		return nil, err
	}

	if cfg.Distribution == "" {
		cfg.Distribution = DefaultDistribution
	}

	if cfg.Component == "" {
		cfg.Component = DefaultComponent
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}

	p := &Publisher{cfg: cfg, base: base}

	p.client = &http.Client{
		Transport: httpclient.Client().Transport,
		Timeout:   cfg.Timeout,
	}

	return p, nil
}

// Publish uploads one package file and, for raw-hosted formats, its detached
// signature sidecars (.sig, .asc) when present next to it. The package result
// is always first.
func (p *Publisher) Publish(ctx context.Context, file string) ([]Result, error) {
	format, ok := DetectFormat(file)
	if !ok {
		return nil, errors.New(errors.ErrTypeValidation, "unsupported package file").
			WithOperation("Publish").
			WithContext("path", file)
	}

	repo := p.cfg.RepoFor(format)
	if repo == "" {
		return nil, errors.Wrap(ErrNoRepository, errors.ErrTypeConfiguration, format).
			WithOperation("Publish").
			WithContext("file", filepath.Base(file))
	}

	sum, size, err := hashFile(file)
	if err != nil {
		return nil, err
	}

	art := artifact{file: file, format: format, repo: repo, sha256: sum, size: size}

	var res Result

	switch format {
	case FormatDeb:
		res, err = p.publishDeb(ctx, art)
	case FormatRPM:
		res, err = p.publishRPM(ctx, art)
	default:
		res, err = p.publishRaw(ctx, art, p.cfg.pathFor(format), filepath.Base(file))
	}

	if err != nil {
		return nil, err
	}

	results := []Result{res}

	if format == FormatAPK || format == FormatPacman {
		sidecars, err := p.publishSidecars(ctx, art)
		if err != nil {
			return results, err
		}

		results = append(results, sidecars...)
	}

	return results, nil
}

// artifact is one local package with its precomputed digest.
type artifact struct {
	file   string
	format string
	repo   string
	sha256 string
	size   int64
}

func (a artifact) result(remote string, st Status) Result {
	return Result{
		File: a.file, Format: a.format, Repo: a.repo, Remote: remote,
		SHA256: a.sha256, Size: a.size, Status: st,
	}
}

func hashFile(file string) (digest string, size int64, err error) {
	f, err := os.Open(file) //nolint:gosec // user-selected artifact
	if err != nil {
		return "", 0, errors.Wrap(err, errors.ErrTypeFileSystem, "cannot open artifact").
			WithOperation("hashFile").
			WithContext("path", file)
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()

	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, errors.Wrap(err, errors.ErrTypeFileSystem, "cannot read artifact").
			WithOperation("hashFile").
			WithContext("path", file)
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// endpoint builds an absolute URL below the server base from already-escaped
// path segments.
func (p *Publisher) endpoint(segments ...string) string {
	u := *p.base

	var raw strings.Builder

	raw.WriteString(strings.TrimRight(p.base.EscapedPath(), "/"))

	for _, s := range segments {
		raw.WriteString("/")
		raw.WriteString(s)
	}

	u.RawPath = raw.String()

	if decoded, err := url.PathUnescape(u.RawPath); err == nil {
		u.Path = decoded
	}

	return u.String()
}

func escapeSegments(rel string) []string {
	var out []string

	for s := range strings.SplitSeq(rel, "/") {
		if s != "" {
			out = append(out, url.PathEscape(s))
		}
	}

	return out
}

// do performs one authenticated request and returns the response; the caller
// closes the body.
func (p *Publisher) do(
	ctx context.Context, method, target string, body io.Reader, size int64, headers map[string]string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeNetwork, "cannot build request").
			WithOperation("do")
	}

	if body != nil {
		req.ContentLength = size
	}

	req.Header.Set("Authorization", "Bearer "+p.cfg.Token.Reveal())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "yap-publish")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req) //nolint:gosec // target is the operator-configured server
	if err != nil {
		// *url.Error carries only method and URL, never the Authorization header.
		return nil, err
	}

	return resp, nil
}

// statusError converts a non-2xx response into an error that the retry layer
// classifies by status code (5xx/408/429 retry, other 4xx do not).
func statusError(resp *http.Response, target string) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	msg := serverMessage(snippet)

	base := &httpclient.HTTPStatusError{Code: resp.StatusCode, URL: redactURL(target)}
	if msg == "" {
		return base
	}

	return errors.Wrap(base, errors.ErrTypeNetwork, msg)
}

// serverMessage extracts {"error": "..."} / {"message": "..."} from an
// OpenArtifact error body, falling back to trimmed plain text.
func serverMessage(body []byte) string {
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	if json.Unmarshal(body, &env) == nil {
		if env.Error != "" {
			return env.Error
		}

		if env.Message != "" {
			return env.Message
		}
	}

	return strings.TrimSpace(string(body))
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	return u.Redacted()
}

// withRetry runs fn under the shared exponential-backoff retry policy.
func withRetry(ctx context.Context, label string, fn func() error) error {
	return httpclient.WithRetry(ctx, label, fn)
}

// ---------------------------------------------------------------------------
// deb / rpm: native multipart upload APIs
// ---------------------------------------------------------------------------

// uploadEnvelope is the OpenArtifact success envelope of the deb/rpm upload
// APIs: {"success": true, "data": {..., "sha256": "..."}}.
type uploadEnvelope struct {
	Data struct {
		SHA256 string `json:"sha256"`
	} `json:"data"`
}

func (p *Publisher) publishDeb(ctx context.Context, a artifact) (Result, error) {
	fields := [][2]string{
		{"distribution", p.cfg.Distribution},
		{"component", p.cfg.Component},
	}

	return p.uploadMultipart(ctx, a, p.endpoint("api", "v1", "repositories", url.PathEscape(a.repo), "debs"), fields)
}

func (p *Publisher) publishRPM(ctx context.Context, a artifact) (Result, error) {
	return p.uploadMultipart(ctx, a, p.endpoint("api", "v1", "repositories", url.PathEscape(a.repo), "packages"), nil)
}

func (p *Publisher) uploadMultipart(
	ctx context.Context, a artifact, target string, fields [][2]string,
) (Result, error) {
	var serverSum string

	conflict := false

	err := withRetry(ctx, redactURL(target), func() error {
		var err error

		serverSum, conflict, err = p.postFile(ctx, a, target, fields)

		return err
	})
	if err != nil {
		return Result{}, publishError(a, err)
	}

	if conflict {
		if err := p.verifyExisting(ctx, a); err != nil {
			return Result{}, err
		}

		logger.Info(i18n.T("logger.publish.info.unchanged"),
			"file", filepath.Base(a.file), "repo", a.repo, "sha256", a.sha256)

		return a.result(target, StatusUnchanged), nil
	}

	if !strings.EqualFold(serverSum, a.sha256) {
		return Result{}, checksumMismatch(a, serverSum)
	}

	logger.Info(i18n.T("logger.publish.info.uploaded"),
		"file", filepath.Base(a.file), "repo", a.repo, "sha256", a.sha256)

	return a.result(target, StatusUploaded), nil
}

// postFile sends one multipart attempt. It streams the file with a known
// Content-Length instead of buffering it. conflict is true on HTTP 409.
func (p *Publisher) postFile(
	ctx context.Context, a artifact, target string, fields [][2]string,
) (serverSum string, conflict bool, err error) {
	f, err := os.Open(a.file) //nolint:gosec // user-selected artifact
	if err != nil {
		return "", false, errors.Wrap(err, errors.ErrTypeFileSystem, "cannot open artifact").
			WithOperation("postFile").
			WithContext("path", a.file)
	}

	defer func() { _ = f.Close() }()

	var head bytes.Buffer

	mw := multipart.NewWriter(&head)

	for _, kv := range fields {
		if err := mw.WriteField(kv[0], kv[1]); err != nil {
			return "", false, err
		}
	}

	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(a.file)))
	partHeader.Set("Content-Type", rawContentType)

	if _, err := mw.CreatePart(partHeader); err != nil {
		return "", false, err
	}

	tail := "\r\n--" + mw.Boundary() + "--\r\n"
	body := io.MultiReader(&head, f, strings.NewReader(tail))
	total := int64(head.Len()) + a.size + int64(len(tail))

	resp, err := p.do(ctx, http.MethodPost, target, body, total,
		map[string]string{"Content-Type": mw.FormDataContentType()})
	if err != nil {
		return "", false, err
	}

	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusConflict:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))

		return "", true, nil
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return "", false, statusError(resp, target)
	}

	var env uploadEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return "", false, errors.Wrap(err, errors.ErrTypeNetwork, "cannot decode upload response").
			WithOperation("postFile")
	}

	return env.Data.SHA256, false, nil
}

// verifyExisting runs after a 409. OpenArtifact answers 409 both for the same
// bytes and for the same name/version with other bytes; only the first case is
// a successful, idempotent re-publish. The repository package listing carries
// each package's SHA-256, so the stored bytes can be compared without
// downloading them.
func (p *Publisher) verifyExisting(ctx context.Context, a artifact) error {
	if name := guessPackageName(a.format, a.file); name != "" {
		found, err := p.listHasSHA(ctx, a, name)
		if err != nil {
			return err
		}

		if found {
			return nil
		}
	}

	// The name is only a filename-derived hint; scan the whole repository
	// before declaring a real conflict.
	found, err := p.listHasSHA(ctx, a, "")
	if err != nil {
		return err
	}

	if !found {
		return errors.Wrap(ErrConflict, errors.ErrTypePackaging, filepath.Base(a.file)).
			WithOperation("verifyExisting").
			WithContext("repo", a.repo)
	}

	return nil
}

type listResponse struct {
	Total    int `json:"total"`
	Packages []struct {
		SHA256 string `json:"sha256"`
	} `json:"packages"`
}

func (p *Publisher) listHasSHA(ctx context.Context, a artifact, name string) (bool, error) {
	for page := 1; page <= maxListPages; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", strconv.Itoa(listPageSize))

		if name != "" {
			q.Set("package_name", name)
		}

		target := p.endpoint("api", "v1", "repositories", url.PathEscape(a.repo), "packages") + "?" + q.Encode()

		var out listResponse

		err := withRetry(ctx, redactURL(target), func() error {
			out = listResponse{}

			return p.getJSON(ctx, target, &out)
		})
		if err != nil {
			return false, publishError(a, err)
		}

		for _, pkg := range out.Packages {
			if strings.EqualFold(pkg.SHA256, a.sha256) {
				return true, nil
			}
		}

		if len(out.Packages) < listPageSize || page*listPageSize >= out.Total {
			return false, nil
		}
	}

	return false, nil
}

func (p *Publisher) getJSON(ctx context.Context, target string, out any) error {
	resp, err := p.do(ctx, http.MethodGet, target, nil, 0, nil)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(resp, target)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork, "cannot decode listing response").
			WithOperation("getJSON")
	}

	return nil
}

// ---------------------------------------------------------------------------
// apk / pacman: hosted raw repositories
// ---------------------------------------------------------------------------

func (p *Publisher) publishRaw(ctx context.Context, a artifact, prefix, name string) (Result, error) {
	rel := path.Join(strings.Trim(prefix, "/"), name)
	segs := append([]string{"repos", "raw", url.PathEscape(a.repo)}, escapeSegments(rel)...)
	target := p.endpoint(segs...)

	remoteSum, exists, err := p.headRaw(ctx, target)
	if err != nil {
		return Result{}, publishError(a, err)
	}

	if exists && strings.EqualFold(remoteSum, a.sha256) {
		logger.Info(i18n.T("logger.publish.info.unchanged"),
			"file", name, "repo", a.repo, "sha256", a.sha256)

		return a.result(rel, StatusUnchanged), nil
	}

	var putSum string

	err = withRetry(ctx, redactURL(target), func() error {
		var err error

		putSum, err = p.putRaw(ctx, a, target)

		return err
	})
	if err != nil {
		return Result{}, publishError(a, err)
	}

	if !strings.EqualFold(putSum, a.sha256) {
		return Result{}, checksumMismatch(a, putSum)
	}

	logger.Info(i18n.T("logger.publish.info.uploaded"),
		"file", name, "repo", a.repo, "sha256", a.sha256)

	return a.result(rel, StatusUploaded), nil
}

// headRaw reports the stored SHA-256 (X-Checksum-Sha256) of a raw file.
func (p *Publisher) headRaw(ctx context.Context, target string) (sum string, exists bool, err error) {
	err = withRetry(ctx, redactURL(target), func() error {
		sum, exists = "", false

		resp, err := p.do(ctx, http.MethodHead, target, nil, 0, nil)
		if err != nil {
			return err
		}

		defer func() { _ = resp.Body.Close() }()

		switch {
		case resp.StatusCode == http.StatusNotFound:
			return nil
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return statusError(resp, target)
		}

		sum, exists = resp.Header.Get("X-Checksum-Sha256"), true

		return nil
	})

	return sum, exists, err
}

func (p *Publisher) putRaw(ctx context.Context, a artifact, target string) (string, error) {
	f, err := os.Open(a.file) //nolint:gosec // user-selected artifact
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "cannot open artifact").
			WithOperation("putRaw").
			WithContext("path", a.file)
	}

	defer func() { _ = f.Close() }()

	resp, err := p.do(ctx, http.MethodPut, target, f, a.size,
		map[string]string{"Content-Type": rawContentType})
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", statusError(resp, target)
	}

	var out struct {
		SHA256 string `json:"sha256"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", errors.Wrap(err, errors.ErrTypeNetwork, "cannot decode upload response").
			WithOperation("putRaw")
	}

	return out.SHA256, nil
}

// publishSidecars uploads NAME.sig / NAME.asc (yap's detached signatures)
// next to a raw-hosted package.
func (p *Publisher) publishSidecars(ctx context.Context, a artifact) ([]Result, error) {
	var results []Result

	for _, ext := range []string{".sig", ".asc"} {
		side := a.file + ext

		sum, size, err := hashFile(side)
		if err != nil {
			if goerrors.Is(err, fs.ErrNotExist) {
				continue
			}

			return results, err
		}

		sa := artifact{file: side, format: a.format, repo: a.repo, sha256: sum, size: size}

		res, err := p.publishRaw(ctx, sa, p.cfg.pathFor(a.format), filepath.Base(side))
		if err != nil {
			return results, err
		}

		results = append(results, res)
	}

	return results, nil
}

// publishError adds the artifact identity to a transport/HTTP error.
func publishError(a artifact, err error) error {
	return errors.Wrap(err, errors.ErrTypeNetwork, "publish failed").
		WithOperation("Publish").
		WithContext("file", filepath.Base(a.file)).
		WithContext("repo", a.repo)
}

func checksumMismatch(a artifact, serverSum string) error {
	return errors.Wrap(ErrChecksumMismatch, errors.ErrTypePackaging, filepath.Base(a.file)).
		WithOperation("Publish").
		WithContext("repo", a.repo).
		WithContext("local_sha256", a.sha256).
		WithContext("server_sha256", serverSum)
}
