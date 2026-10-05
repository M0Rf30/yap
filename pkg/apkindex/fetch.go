// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex

import (
	"archive/tar"
	"context"
	"crypto/sha1" //nolint:gosec
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/gzip"
	"golang.org/x/sync/errgroup"

	apperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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

	logger.Info(i18n.T("logger.apkindex.info.updating_indexes"), "repos", len(repos),
		"arch", arch)

	idx := NewIndex()
	succeeded := 0

	// Phase 1: download all APKINDEX tarballs in parallel (network-bound;
	// destinations are distinct cache files keyed by URL hash).
	type fetchResult struct {
		indexURL  string
		cachePath string
		err       error
	}

	results := make([]fetchResult, len(repos))

	g := new(errgroup.Group)
	g.SetLimit(4)

	for i, repo := range repos {
		g.Go(func() error {
			indexURL := repo.URL + "/" + arch + "/APKINDEX.tar.gz"
			cachePath := filepath.Join(apkCacheDir, "APKINDEX."+sha1Hex(indexURL)+".tar.gz")

			logger.Debug(i18n.T("logger.apkindex.debug.fetching_repo"), "url", repo.URL, "arch", arch)

			results[i] = fetchResult{
				indexURL:  indexURL,
				cachePath: cachePath,
				err:       downloadFile(ctx, indexURL, cachePath, maxAPKIndexBytes),
			}

			return nil
		})
	}

	_ = g.Wait()

	// Phase 2: parse sequentially — Index mutation is not concurrency-safe
	// and parsing is cheap compared to the network fetch.
	for i, repo := range repos {
		res := results[i]
		if res.err != nil {
			// Log warning and continue with other repos.
			logger.Warn(i18n.T("logger.apkindex.warn.fetch_failed"), "url", res.indexURL, "error", res.err)
			continue
		}

		var sizeBytes int64
		if fi, statErr := os.Stat(res.cachePath); statErr == nil {
			sizeBytes = fi.Size()
		}

		if err := loadIndexTarball(idx, res.cachePath, repo.URL); err != nil {
			logger.Warn(i18n.T("logger.apkindex.warn.parse_failed"), "path", res.cachePath, "error", err)

			continue
		}

		logger.Info(i18n.T("logger.apkindex.info.repo_fetched"), "url", repo.URL, "bytes", sizeBytes)

		succeeded++
	}

	pkgs, caps := idx.Stats()
	logger.Info(i18n.T("logger.apkindex.info.indexes_loaded"), "repos_succeeded", succeeded,
		"repos_total", len(repos),
		"packages", pkgs,
		"capabilities", caps)

	if succeeded == 0 {
		return nil, apperrors.New(apperrors.ErrTypeNetwork,
			"no APK repository index could be loaded").
			WithOperation("Update").
			WithContext("repos", len(repos))
	}

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

	if !safeAPKComponent(pkg.Name) || !safeAPKComponent(pkg.Version) ||
		!safeAPKComponent(pkg.Arch) {
		return "", apperrors.New(apperrors.ErrTypeValidation,
			"unsafe package name, version or arch in index").
			WithOperation("DownloadPackage").
			WithContext("package", name)
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
// Each download goes through httpclient.FetchToFile (retry, timeout, size cap).
func (idx *Index) DownloadPackages(ctx context.Context, destDir string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	jobs, pathMap, err := idx.buildAPKDownloadRequests(destDir, names)
	if err != nil {
		return nil, err
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(apkDownloadConcurrency, len(jobs)))

	for _, job := range jobs {
		g.Go(func() error {
			limit := int64(maxAPKPackageBytes)
			if job.size > 0 && job.size < limit {
				limit = job.size
			}

			if dlErr := downloadFile(gctx, job.url, job.dest, limit); dlErr != nil {
				return apperrors.Wrap(dlErr, apperrors.ErrTypeNetwork, "failed to download package").
					WithOperation("DownloadPackages").
					WithContext("filename", filepath.Base(job.dest))
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return pathMap, nil
}

// apkDownload describes one package download.
type apkDownload struct {
	url  string
	dest string
	size int64
}

// buildAPKDownloadRequests builds the download jobs for each package name.
// Returns the jobs and a pre-built name→destPath map (populated before any HTTP).
func (idx *Index) buildAPKDownloadRequests(
	destDir string, names []string,
) ([]apkDownload, map[string]string, error) {
	jobs := make([]apkDownload, 0, len(names))
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

		if !safeAPKComponent(pkg.Name) || !safeAPKComponent(pkg.Version) ||
			!safeAPKComponent(pkg.Arch) {
			return nil, nil, apperrors.New(apperrors.ErrTypeValidation,
				"unsafe package name, version or arch in index").
				WithOperation("buildAPKDownloadRequests").
				WithContext("package", name)
		}

		filename := pkg.Name + "-" + pkg.Version + ".apk"
		destPath := filepath.Join(destDir, filename)

		jobs = append(jobs, apkDownload{
			url:  pkg.RepoBaseURL + "/" + pkg.Arch + "/" + filename,
			dest: destPath,
			size: pkg.Size,
		})
		pathMap[name] = destPath
	}

	return jobs, pathMap, nil
}

// safeAPKComponent reports whether s is usable as a single path/URL segment
// of a download filename (no separators, not "." or "..", no NUL).
func safeAPKComponent(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}

	return !strings.ContainsAny(s, "/\\\x00")
}
