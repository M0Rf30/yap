package apkindex

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1" // #nosec G505 -- SHA1 required by APK format
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const apkCacheDir = "/var/cache/apk"

// maxAPKIndexBytes caps an APKINDEX.tar.gz download. Real Alpine indexes are
// ~5 MB; 100 MB is plenty of slack and still defends against an unbounded
// stream.
const maxAPKIndexBytes = 100 << 20

// maxAPKPackageBytes caps an individual .apk package at 1 GiB. The largest
// Alpine packages (e.g. linux-edge) are well under 100 MB.
const maxAPKPackageBytes = 1 << 30

// Update fetches APKINDEX.tar.gz from every repo in /etc/apk/repositories,
// writes the parsed indexes into the cache dir, and returns an Index ready
// for lookups. Replaces "apk update". The returned Index is cached globally
// so Install can reuse it without re-fetching.
func Update(ctx context.Context) (*Index, error) {
	repos, err := LoadRepos()
	if err != nil {
		return nil, fmt.Errorf("apkindex: load repos: %w", err)
	}

	arch := DetectArch()
	if arch == "" {
		return nil, fmt.Errorf("apkindex: could not detect APK architecture")
	}

	if err := os.MkdirAll(apkCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("apkindex: mkdir cache: %w", err)
	}

	logger.Info("apkindex: updating indexes",
		"repos", len(repos),
		"arch", arch)

	idx := NewIndex()
	succeeded := 0

	for _, repo := range repos {
		indexURL := repo.URL + "/" + arch + "/APKINDEX.tar.gz"
		cachePath := filepath.Join(apkCacheDir, "APKINDEX."+sha1Hex(indexURL)+".tar.gz")

		logger.Debug("apkindex: fetching repo", "url", repo.URL, "arch", arch)

		if err := downloadFile(ctx, indexURL, cachePath, maxAPKIndexBytes); err != nil {
			// Log warning and continue with other repos.
			logger.Warn("apkindex: fetch failed",
				"url", indexURL, "error", err)

			continue
		}

		var sizeBytes int64
		if fi, statErr := os.Stat(cachePath); statErr == nil {
			sizeBytes = fi.Size()
		}

		if err := loadIndexTarball(idx, cachePath, repo.URL); err != nil {
			logger.Warn("apkindex: parse failed",
				"path", cachePath, "error", err)

			continue
		}

		logger.Info("apkindex: repo fetched",
			"url", repo.URL, "bytes", sizeBytes)

		succeeded++
	}

	pkgs, caps := idx.Stats()
	logger.Info("apkindex: indexes loaded",
		"repos_succeeded", succeeded,
		"repos_total", len(repos),
		"packages", pkgs,
		"capabilities", caps)

	// Cache the index globally so Install can reuse it.
	globalIndex.Store(idx)

	return idx, nil
}

// sha1Hex returns the hex-encoded SHA1 hash of a string.
func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s)) // #nosec G401 -- SHA1 required by APK format
	return fmt.Sprintf("%x", h)
}

// downloadFile downloads a file from url and saves it to destPath. The
// response is streamed through an io.LimitReader bounded at maxBytes so a
// malicious or buggy mirror cannot OOM the build.
func downloadFile(ctx context.Context, url, destPath string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("apkindex: new request: %w", err)
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return fmt.Errorf("apkindex: http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := httpclient.CheckStatus(resp, url); err != nil {
		return err
	}

	// Cross-check the advertised Content-Length first (cheap fail-fast).
	if resp.ContentLength > 0 && maxBytes > 0 && resp.ContentLength > maxBytes {
		return fmt.Errorf("apkindex: content too large: %d bytes (cap %d)",
			resp.ContentLength, maxBytes)
	}

	// Write to a temp file first, then rename.
	tmpPath := destPath + ".tmp"

	f, err := os.Create(tmpPath) // #nosec G304 — destPath is constructed from URL
	if err != nil {
		return fmt.Errorf("apkindex: create temp: %w", err)
	}

	defer func() { _ = f.Close() }()

	// LimitReader+1 trick: read one byte past the cap so we can detect
	// servers that lie about Content-Length.
	body := io.LimitReader(resp.Body, maxBytes+1)

	written, err := io.Copy(f, body)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("apkindex: copy: %w", err)
	}

	if written > maxBytes {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("apkindex: body exceeded %d-byte cap", maxBytes)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("apkindex: close temp: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("apkindex: rename: %w", err)
	}

	return nil
}

// loadIndexTarball opens an APKINDEX.tar.gz, finds the APKINDEX entry,
// and feeds it to idx.ParseIndex.
func loadIndexTarball(idx *Index, path, repoBaseURL string) error {
	f, err := os.Open(path) // #nosec G304 -- cache path
	if err != nil {
		return fmt.Errorf("apkindex: open tarball: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("apkindex: gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("apkindex: tar read: %w", err)
		}

		if hdr.Name == "APKINDEX" {
			return idx.ParseIndex(tr, repoBaseURL)
		}
	}
}

// DownloadPackage downloads a .apk file to destDir and returns its path.
func (idx *Index) DownloadPackage(ctx context.Context, destDir, name string) (string, error) {
	pkg, ok := idx.Lookup(name)
	if !ok {
		// Try virtual.
		if vp, ok := idx.ResolveVirtual(name); ok {
			pkg = vp
		} else {
			return "", fmt.Errorf("apkindex: package %q not found", name)
		}
	}

	filename := pkg.Name + "-" + pkg.Version + ".apk"
	url := pkg.RepoBaseURL + "/" + pkg.Arch + "/" + filename
	destPath := filepath.Join(destDir, filename)

	if err := downloadFile(ctx, url, destPath, maxAPKPackageBytes); err != nil {
		return "", fmt.Errorf("apkindex: download %s: %w", filename, err)
	}

	return destPath, nil
}

// DownloadPackages downloads multiple packages and returns a map of name → path.
func (idx *Index) DownloadPackages(ctx context.Context, destDir string, names []string) (map[string]string, error) {
	result := make(map[string]string)

	for _, name := range names {
		path, err := idx.DownloadPackage(ctx, destDir, name)
		if err != nil {
			return nil, err
		}

		result[name] = path
	}

	return result, nil
}
