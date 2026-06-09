package dnfcache

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// downloadAndInstall downloads the resolved packages concurrently and installs
// them via "rpm --install --nodeps".
func (c *Cache) downloadAndInstall(ctx context.Context, pkgs []*PackageInfo) error {
	tmpDir, err := os.MkdirTemp("", "dnfcache-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary directory").
			WithOperation("downloadAndInstall")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	var totalBytes int64
	for _, p := range pkgs {
		totalBytes += p.Size
	}

	logger.Info(i18n.T("logger.dnfcache.info.downloading_packages"), "count", len(pkgs),
		"total_bytes", totalBytes)

	paths, err := c.downloadAll(ctx, pkgs, tmpDir)
	if err != nil {
		return err
	}

	return rpmInstall(ctx, paths)
}

// downloadAll downloads pkgs concurrently into destDir and returns the local
// file paths in the same order as pkgs.
func (c *Cache) downloadAll(ctx context.Context, pkgs []*PackageInfo, destDir string) ([]string, error) {
	type result struct {
		idx  int
		path string
		err  error
	}

	concurrency := min(min(runtime.GOMAXPROCS(0), 4), len(pkgs))

	type job struct {
		idx int
		pkg *PackageInfo
	}

	jobCh := make(chan job, len(pkgs))
	for i, p := range pkgs {
		jobCh <- job{idx: i, pkg: p}
	}

	close(jobCh)

	resCh := make(chan result, len(pkgs))

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for j := range jobCh {
				path, err := downloadRPM(ctx, j.pkg, destDir)
				resCh <- result{idx: j.idx, path: path, err: err}
			}
		}()
	}

	wg.Wait()
	close(resCh)

	paths := make([]string, len(pkgs))

	for res := range resCh {
		if res.err != nil {
			return nil, res.err
		}

		paths[res.idx] = res.path
	}

	return paths, nil
}

// DownloadRPM downloads a single .rpm package to destDir, verifying SHA256
// against the cached metadata. Exported wrapper around the internal helper so
// callers outside the package (e.g. pkg/dnfinstall) can reuse the same
// download path used by Install.
func DownloadRPM(ctx context.Context, pkg *PackageInfo, destDir string) (string, error) {
	return downloadRPM(ctx, pkg, destDir)
}

// downloadRPM downloads a single .rpm to destDir, verifying SHA256. When
// the package's repo was indexed via a mirrorlist, every resolved mirror
// is tried in order before giving up; transient HTTP failures on a single
// mirror are retried by the httpclient retry policy.
func downloadRPM(ctx context.Context, pkg *PackageInfo, destDir string) (string, error) {
	baseURLs, err := packageBaseURLs(ctx, pkg)
	if err != nil {
		return "", err
	}

	dest := filepath.Join(destDir, filepath.Base(pkg.LocationHref))

	var lastErr error

	for i, baseURL := range baseURLs {
		err := downloadVerified(ctx, baseURL+pkg.LocationHref, dest, pkg.SHA256)
		if err == nil {
			logger.Debug(i18n.T("logger.dnfcache.debug.downloaded_rpm"), "package", pkg.Name,
				"dest", dest)

			return dest, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			break
		}

		if i < len(baseURLs)-1 {
			logger.Warn(i18n.T("logger.dnfcache.warn.mirror_failed_trying_next"),
				"package", pkg.Name,
				"mirror", baseURL,
				"remaining", len(baseURLs)-i-1,
				"error", lastErr)
		}
	}

	return "", errors.Wrap(lastErr, errors.ErrTypeNetwork, "failed to download package").
		WithOperation("downloadRPM").
		WithContext("package", pkg.Name)
}

// packageBaseURLs returns the candidate base URLs (trailing slash
// included) for downloading pkg. A "mirrorlist:" placeholder set by
// loadFromDisk (no baseurl available at index-load time) is resolved to
// the full mirror candidate list.
func packageBaseURLs(ctx context.Context, pkg *PackageInfo) ([]string, error) {
	rest, ok := strings.CutPrefix(pkg.BaseURL, "mirrorlist:")
	if !ok {
		return []string{pkg.BaseURL}, nil
	}

	mirrors, err := resolveMirrors(ctx, rest)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeNetwork, "failed to resolve mirrorlist").
			WithOperation("downloadRPM").
			WithContext("package", pkg.Name)
	}

	urls := make([]string, 0, len(mirrors))
	for _, m := range mirrors {
		urls = append(urls, strings.TrimSuffix(m, "/")+"/")
	}

	return urls, nil
}

// rpmInstall installs the given .rpm files via "rpm --install --nodeps".
// --nodeps is used because we have already resolved the dependency closure.
func rpmInstall(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	args := append([]string{"--install", "--nodeps"}, paths...)

	cmd := exec.CommandContext(ctx, "rpm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info(i18n.T("logger.dnfcache.info.installing_rpm_packages"), "count", len(paths))

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to install RPM packages").
			WithOperation("rpmInstall")
	}

	return nil
}

// logUnresolved emits a warning for each package name that could not be
// resolved in the index.
func logUnresolved(names []string) {
	for _, name := range names {
		logger.Warn(i18n.T("logger.dnfcache.warn.unresolvable_dependency"), "package", name)
	}
}
