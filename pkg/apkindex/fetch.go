package apkindex

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cavaliergopher/grab/v3"

	apperrors "github.com/M0Rf30/yap/v2/pkg/errors"
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

// apkDownloadConcurrency caps the number of parallel .apk downloads handed
// to grab.Client.DoBatch. Matches aptcache's downloadConcurrency.
const apkDownloadConcurrency = 6

// Update fetches APKINDEX.tar.gz from every repo in /etc/apk/repositories,
// writes the parsed indexes into the cache dir, and returns an Index ready
// for lookups. Replaces "apk update". The returned Index is cached globally
// so Install can reuse it without re-fetching.
func Update(ctx context.Context) (*Index, error) {
	repos, err := LoadRepos()
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.ErrTypeConfiguration, "load repos").
			WithOperation("Update")
	}

	arch := DetectArch()
	if arch == "" {
		return nil, apperrors.New(apperrors.ErrTypeConfiguration, "could not detect APK architecture").
			WithOperation("Update")
	}

	if err := os.MkdirAll(apkCacheDir, 0o755); err != nil {
		return nil, apperrors.Wrap(err, apperrors.ErrTypeFileSystem, "mkdir cache").
			WithOperation("Update")
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
	h := sha1.Sum([]byte(s)) //nolint:gosec
	return fmt.Sprintf("%x", h)
}

// downloadFile downloads a file from url and saves it to destPath. The
// response is streamed through an io.LimitReader bounded at maxBytes so a
// malicious or buggy mirror cannot OOM the build.
func downloadFile(ctx context.Context, url, destPath string, maxBytes int64) error {
	if err := httpclient.FetchToFile(ctx, url, destPath, maxBytes); err != nil {
		return apperrors.Wrap(err, apperrors.ErrTypeNetwork, "download file").
			WithOperation("downloadFile").
			WithContext("url", url)
	}

	return nil
}

// loadIndexTarball opens an APKINDEX.tar.gz, finds the APKINDEX entry,
// and feeds it to idx.ParseIndex.
func loadIndexTarball(idx *Index, path, repoBaseURL string) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return apperrors.Wrap(err, apperrors.ErrTypeFileSystem, "open tarball").
			WithOperation("loadIndexTarball").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return apperrors.Wrap(err, apperrors.ErrTypeParser, "gzip reader").
			WithOperation("loadIndexTarball").
			WithContext("path", path)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return apperrors.Wrap(err, apperrors.ErrTypeParser, "tar read").
				WithOperation("loadIndexTarball").
				WithContext("path", path)
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
			return "", apperrors.New(apperrors.ErrTypePackaging, "package not found").
				WithOperation("DownloadPackage").
				WithContext("package", name)
		}
	}

	filename := pkg.Name + "-" + pkg.Version + ".apk"
	url := pkg.RepoBaseURL + "/" + pkg.Arch + "/" + filename
	destPath := filepath.Join(destDir, filename)

	if err := downloadFile(ctx, url, destPath, maxAPKPackageBytes); err != nil {
		return "", apperrors.Wrap(err, apperrors.ErrTypeNetwork, "download package").
			WithOperation("DownloadPackage").
			WithContext("filename", filename)
	}

	return destPath, nil
}

// DownloadPackages downloads multiple packages in parallel and returns a map of name → path.
// Uses cavaliergopher/grab for concurrent downloads.
func (idx *Index) DownloadPackages(ctx context.Context, destDir string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	requests, pathMap, err := idx.buildAPKDownloadRequests(ctx, destDir, names)
	if err != nil {
		return nil, err
	}

	workers := min(apkDownloadConcurrency, len(requests))
	client := grab.NewClient()
	client.UserAgent = "YAP/2 (apkindex)"

	respCh := client.DoBatch(workers, requests...)

	var firstErr error

	for resp := range respCh {
		if err := resp.Err(); err != nil && firstErr == nil {
			firstErr = apperrors.Wrap(err, apperrors.ErrTypeNetwork, "failed to download package").
				WithOperation("DownloadPackages").
				WithContext("filename", filepath.Base(resp.Filename))
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return pathMap, nil
}

// buildAPKDownloadRequests builds grab.Request objects for each package name.
// Returns the requests and a pre-built name→destPath map (populated before any HTTP).
func (idx *Index) buildAPKDownloadRequests(
	ctx context.Context, destDir string, names []string,
) ([]*grab.Request, map[string]string, error) {
	requests := make([]*grab.Request, 0, len(names))
	pathMap := make(map[string]string, len(names))

	for _, name := range names {
		pkg, ok := idx.Lookup(name)
		if !ok {
			if vp, ok2 := idx.ResolveVirtual(name); ok2 {
				pkg = vp
			} else {
				return nil, nil, apperrors.New(apperrors.ErrTypePackaging, "package not found").
					WithOperation("buildAPKDownloadRequests").
					WithContext("package", name)
			}
		}

		filename := pkg.Name + "-" + pkg.Version + ".apk"
		pkgURL := pkg.RepoBaseURL + "/" + pkg.Arch + "/" + filename
		destPath := filepath.Join(destDir, filename)

		req, err := grab.NewRequest(destPath, pkgURL)
		if err != nil {
			return nil, nil, apperrors.Wrap(err, apperrors.ErrTypeNetwork, "build request").
				WithOperation("buildAPKDownloadRequests").
				WithContext("package", name)
		}

		req = req.WithContext(ctx)

		if pkg.Size > 0 {
			req.Size = pkg.Size
		}

		requests = append(requests, req)
		pathMap[name] = destPath
	}

	return requests, pathMap, nil
}
