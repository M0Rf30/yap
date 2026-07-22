// download.go: .deb download, verification, and closure helpers.

package aptcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cavaliergopher/grab/v3"

	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// DownloadClosure resolves the transitive closure of the supplied seed
// package names, downloads every resulting .deb into destDir, and returns
// the resolved PackageInfo slice in dependency order (deps before
// dependents) plus the list of names that could not be resolved.
//
// Behaviour:
//   - Packages already marked Installed are skipped (their dependencies are
//     still walked so transitive runtime deps reachable only through an
//     installed library still get pulled in).
//   - Virtual packages are resolved to their first concrete provider via
//     the reverse-Provides index.
//   - "foo | bar" alternatives resolve to the first option.
//   - Architecture / version qualifiers on seed names are stripped before
//     lookup; see Lookup for the bare-name contract.
//   - Cycles are short-circuited by ResolveDeps' internal `seen` map.
//
// This is the helper that callers should reach for when they need
// "everything required to actually use these packages on the target
// filesystem" — most importantly, cross-build runtime dep extraction in
// pkg/builders/common.DownloadAndExtractCrossDeps, where missing
// transitive deps cause cross-link failures like:
//
//	ld: warning: libvpx.so.12, needed by libavcodec.so, not found
//
// because PKGBUILDs only declare the direct dep (vendor-ffmpeg) while
// the transitive arch-specific libs (vendor-libvpx, vendor-x264) are
// not surfaced unless we walk the dep graph ourselves.
func (c *Cache) DownloadClosure(
	ctx context.Context, destDir string, seeds []string,
) (resolved []*PackageInfo, unresolved []string, err error) {
	resolved, unresolved, err = c.ResolveDeps(seeds)
	if err != nil {
		return nil, nil, err
	}

	if len(resolved) == 0 {
		return resolved, unresolved, nil
	}

	names := make([]string, 0, len(resolved))
	for _, p := range resolved {
		if p.Architecture == "" || p.Architecture == archAll {
			names = append(names, p.Name)
		} else {
			names = append(names, p.Name+":"+p.Architecture)
		}
	}

	if err := c.Download(ctx, destDir, names); err != nil {
		return resolved, unresolved, err
	}

	return resolved, unresolved, nil
}

// downloadConcurrency caps the number of parallel .deb downloads handed
// to grab.Client.DoBatch. Each mirror tolerates a handful of concurrent
// connections; 6 is enough to saturate a typical 100-1000 Mbit/s link
// without being rude to the mirror.
const downloadConcurrency = 6

// Download fetches the named packages into destDir using the apt package
// index metadata (Filename, SHA256, Size, BaseURL fields).
//
// Implementation: uses cavaliergopher/grab (the same library yap's
// pkg/download uses for source downloads). grab gives us for free:
//
//   - Concurrent batched downloads (DoBatch with a fixed worker pool).
//   - HTTP Range / resume on partially-downloaded files (so an interrupted
//     `yap build` doesn't re-fetch hundreds of MB).
//   - In-stream SHA-256 verification via Request.SetChecksum, with
//     delete-on-error so a corrupt .deb never lingers at destDir.
//
// Performance: a 100-package closure that took ~30s with sequential
// net/http drops to ~5-8s against archive.ubuntu.com.
//
// Returns an error if any package is not found in the index or any
// download fails. All downloads continue until completion (or context
// cancel); errors are aggregated and the first one returned. Partial
// files left by failed downloads are removed by grab itself.
//
// Most callers should prefer DownloadClosure, which performs transitive
// resolution before downloading. Use Download directly only when you
// already have an explicit, pre-resolved list of package names.
func (c *Cache) Download(ctx context.Context, destDir string, pkgs []string) error {
	client := grab.NewClient()
	client.UserAgent = "YAP/2 (aptcache)"

	return c.downloadWithClient(ctx, client, destDir, pkgs)
}

// downloadWithClient is Download's implementation, parameterized on the
// grab.Client so tests can inject a fake HTTPClient/transport to exercise
// the https-fallback path deterministically, without real network I/O.
func (c *Cache) downloadWithClient(
	ctx context.Context, client *grab.Client, destDir string, pkgs []string,
) error {
	if len(pkgs) == 0 {
		return nil
	}

	jobs, err := c.resolveDownloadJobs(pkgs)
	if err != nil {
		return err
	}

	requests, err := c.buildDownloadRequests(ctx, destDir, jobs)
	if err != nil {
		return err
	}

	responses := runDownloadBatch(client, requests)
	responses = c.retryHTTPSFallback(ctx, client, destDir, jobs, responses)

	var firstEr error

	for _, resp := range responses {
		if err := resp.Err(); err != nil && firstEr == nil {
			firstEr = errors.Wrap(err, errors.ErrTypeNetwork, "failed to download package").
				WithOperation("Download").
				WithContext("filename", filepath.Base(resp.Filename))
		}
	}

	return firstEr
}

// runDownloadBatch runs requests concurrently (capped at
// downloadConcurrency workers) and returns the responses in the same
// order as requests. grab.Client.Do rewraps each Request in a new
// context-bound copy internally, so Response.Request is NOT the same
// pointer callers submitted — index-stable dispatch (not a
// pointer-identity map) is required to pair a failure back to its
// originating package for the https-fallback pass.
func runDownloadBatch(client *grab.Client, requests []*grab.Request) []*grab.Response {
	if len(requests) == 0 {
		return nil
	}

	responses := make([]*grab.Response, len(requests))
	workers := min(downloadConcurrency, len(requests))
	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)

		sem <- struct{}{}

		go func(i int, req *grab.Request) {
			defer wg.Done()
			defer func() { <-sem }()

			resp := client.Do(req)
			resp.Wait()
			responses[i] = resp
		}(i, req)
	}

	wg.Wait()

	return responses
}

// retryHTTPSFallback rebuilds and re-runs, over https, any download that
// failed with a retryable network error while declared as plain http.
// Some networks (corporate egress proxies, sandboxed CI runners) reset or
// drop port-80 connections outright while leaving 443 untouched; the
// declared mirror serves identical content on both schemes, so escalating
// here recovers automatically instead of failing the build over a scheme
// choice baked into an old sources.list entry.
//
// Each failing package gets exactly one https attempt — this is a
// scheme-level escape hatch, not a general retry policy for flaky
// mirrors.
func (c *Cache) retryHTTPSFallback(
	ctx context.Context, client *grab.Client, destDir string,
	jobs []*downloadJob, responses []*grab.Response,
) []*grab.Response {
	var (
		idxs     []int
		requests []*grab.Request
	)

	for i, resp := range responses {
		err := resp.Err()
		if err == nil || !download.IsRetryableGrabError(err) {
			continue
		}

		httpsURL, ok := httpclient.UpgradeToHTTPS(resp.Request.URL().String())
		if !ok {
			continue
		}

		// A bad-length failure leaves an unresumable partial at Filename;
		// clear it so the https attempt starts from scratch instead of
		// resuming (and re-failing against) the same invalid bytes.
		if stderrors.Is(err, grab.ErrBadLength) {
			_ = os.Remove(resp.Filename)
		}

		req, buildErr := c.buildRequest(ctx, destDir, jobs[i], httpsURL)
		if buildErr != nil {
			continue
		}

		idxs = append(idxs, i)
		requests = append(requests, req)
	}

	if len(requests) == 0 {
		return responses
	}

	fallback := runDownloadBatch(client, requests)
	for i, idx := range idxs {
		responses[idx] = fallback[i]
	}

	return responses
}

// downloadJob pairs a resolved package name with its apt-index metadata so
// a failed request can be rebuilt against a different URL (https
// fallback) without re-parsing the package list.
type downloadJob struct {
	name string
	info PackageInfo
}

// resolveDownloadJobs resolves each package name to its apt-index metadata
// up front, so a missing package or missing BaseURL surfaces before any
// HTTP is attempted.
func (c *Cache) resolveDownloadJobs(pkgs []string) ([]*downloadJob, error) {
	jobs := make([]*downloadJob, 0, len(pkgs))

	for _, pkg := range pkgs {
		name := pkg
		// Strip version constraint "(>= 1.0)" but preserve any arch qualifier.
		if i := strings.Index(name, "("); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}

		name = strings.TrimSpace(name)

		info, ok := c.Lookup(name)
		if !ok || info.Filename == "" {
			return nil, errors.New(errors.ErrTypeValidation, "package not found in apt index").
				WithOperation("resolveDownloadJobs").
				WithContext("package", name)
		}

		if info.BaseURL == "" {
			return nil, errors.New(errors.ErrTypeValidation, "package has no BaseURL").
				WithOperation("resolveDownloadJobs").
				WithContext("package", name)
		}

		jobs = append(jobs, &downloadJob{name: name, info: info})
	}

	return jobs, nil
}

// buildDownloadRequests turns each resolved job into a configured grab
// Request against its index-declared BaseURL.
func (c *Cache) buildDownloadRequests(
	ctx context.Context, destDir string, jobs []*downloadJob,
) ([]*grab.Request, error) {
	requests := make([]*grab.Request, 0, len(jobs))

	for _, job := range jobs {
		pkgURL := strings.TrimSuffix(job.info.BaseURL, "/") + "/" + job.info.Filename

		req, err := c.buildRequest(ctx, destDir, job, pkgURL)
		if err != nil {
			return nil, err
		}

		requests = append(requests, req)
	}

	return requests, nil
}

// buildRequest configures a grab.Request for job against pkgURL, wiring in
// the apt-index-supplied size and SHA-256 checksum. Factored out of
// buildDownloadRequests so retryHTTPSFallback can rebuild the same request
// against a different URL without re-deriving checksum/size from scratch.
func (c *Cache) buildRequest(
	ctx context.Context, destDir string, job *downloadJob, pkgURL string,
) (*grab.Request, error) {
	destFile := filepath.Join(destDir, filepath.Base(job.info.Filename))

	req, err := grab.NewRequest(destFile, pkgURL)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeInternal, "failed to build download request").
			WithOperation("buildRequest").
			WithContext("package", job.name)
	}

	req = req.WithContext(ctx)
	if job.info.Size > 0 {
		req.Size = job.info.Size
	}

	if job.info.SHA256 != "" {
		sum, decErr := hex.DecodeString(job.info.SHA256)
		if decErr == nil {
			// SetChecksum(hash, sum, deleteOnError=true):
			//   - streaming SHA-256 against `sum`;
			//   - delete the on-disk file if the hash mismatches,
			//     so a failed download never leaves a corrupt
			//     artifact at destFile.
			req.SetChecksum(sha256.New(), sum, true)
		}
	}

	return req, nil
}

// maxDebBytes caps an individual .deb download. Real Debian packages top
// out around 500 MB (e.g. texlive-full); 2 GiB is generous head-room while
// still defending against an unbounded mirror stream.
const maxDebBytes int64 = 2 << 30

// downloadAndVerify downloads a file from pkgURL to destFile and verifies its
// SHA-256 checksum and size.
//
// The download is streamed through a size-capped io.LimitReader, written
// first to "<destFile>.tmp", hashed inline, and only renamed onto destFile
// after every verification step succeeds. A failed verification leaves no
// partial file at destFile — preventing callers from mistaking a corrupt
// stub for a verified package.
// Transient network failures (connection reset, mid-body EOF, HTTP 5xx)
// are retried per the httpclient retry policy.
func downloadAndVerify(ctx context.Context, pkgURL, destFile, expectedSHA256 string, expectedSize int64) error {
	return httpclient.WithRetry(ctx, pkgURL, func() error {
		return downloadAndVerifyOnce(ctx, pkgURL, destFile, expectedSHA256, expectedSize)
	})
}

// downloadAndVerifyOnce performs a single download + verify attempt.
func downloadAndVerifyOnce(
	ctx context.Context, pkgURL, destFile, expectedSHA256 string, expectedSize int64,
) error {
	resp, err := startDownload(ctx, pkgURL)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if err := preflightContentLength(resp, pkgURL, expectedSize); err != nil {
		return err
	}

	tmpFile := destFile + ".tmp"

	got, n, err := streamToTmp(resp, tmpFile)
	if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if err := verifySizeAndHash(n, got, expectedSize, expectedSHA256, pkgURL); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if err := os.Rename(tmpFile, destFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	return nil
}

// startDownload issues the GET and validates the response status.
func startDownload(ctx context.Context, pkgURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkgURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return nil, err
	}

	if err := httpclient.CheckStatus(resp, pkgURL); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	return resp, nil
}

// preflightContentLength fails fast if the server advertised a length that
// either exceeds the cap or contradicts the apt-index's expected size.
func preflightContentLength(resp *http.Response, pkgURL string, expectedSize int64) error {
	if resp.ContentLength <= 0 {
		return nil
	}

	if resp.ContentLength > maxDebBytes {
		return errors.New(errors.ErrTypeValidation, "response body too large").
			WithOperation("checkContentLength").
			WithContext("url", pkgURL).
			WithContext("size", resp.ContentLength).
			WithContext("cap", maxDebBytes)
	}

	if expectedSize > 0 && resp.ContentLength != expectedSize {
		return errors.New(errors.ErrTypeValidation, "Content-Length mismatch").
			WithOperation("checkContentLength").
			WithContext("url", pkgURL).
			WithContext("got", resp.ContentLength).
			WithContext("expected", expectedSize)
	}

	return nil
}

// streamToTmp copies the response body into tmpFile, computing the SHA-256
// inline. Returns the hex-encoded hash and the byte count actually
// written. The LimitReader+1 trick detects servers that lie about
// Content-Length by yielding one byte beyond the cap.
func streamToTmp(resp *http.Response, tmpFile string) (hashHex string, written int64, err error) {
	f, err := os.Create(tmpFile) //nolint:gosec
	if err != nil {
		return "", 0, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	body := io.LimitReader(resp.Body, maxDebBytes+1)

	n, err := io.Copy(w, body)
	if err != nil {
		return "", n, err
	}

	if err := f.Sync(); err != nil {
		return "", n, err
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// verifySizeAndHash checks the streamed size against the cap and the
// expected size, and the hash against the expected SHA-256.
func verifySizeAndHash(n int64, gotHash string, expectedSize int64, expectedSHA256, pkgURL string) error {
	if n > maxDebBytes {
		return errors.New(errors.ErrTypeValidation, "downloaded size exceeded cap").
			WithOperation("verifySizeAndHash").
			WithContext("url", pkgURL).
			WithContext("size", n).
			WithContext("cap", maxDebBytes)
	}

	if expectedSize > 0 && n != expectedSize {
		return errors.New(errors.ErrTypeValidation, "size mismatch").
			WithOperation("verifySizeAndHash").
			WithContext("got", n).
			WithContext("expected", expectedSize)
	}

	if expectedSHA256 != "" && gotHash != expectedSHA256 {
		return errors.New(errors.ErrTypeValidation, "SHA256 mismatch").
			WithOperation("verifySizeAndHash").
			WithContext("got", gotHash).
			WithContext("expected", expectedSHA256)
	}

	return nil
}
