package dnfcache

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
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

	logger.Info("dnfcache: downloading packages",
		"count", len(pkgs),
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

// downloadRPM downloads a single .rpm to destDir, verifying SHA256.
func downloadRPM(ctx context.Context, pkg *PackageInfo, destDir string) (string, error) {
	baseURL := pkg.BaseURL

	// Resolve mirrorlist placeholder set by loadFromDisk when no baseurl was
	// available at index-load time.
	if rest, ok := strings.CutPrefix(baseURL, "mirrorlist:"); ok {
		resolved, err := resolveMirrorList(ctx, rest)
		if err != nil {
			return "", errors.Wrap(err, errors.ErrTypeNetwork, "failed to resolve mirrorlist").
				WithOperation("downloadRPM").
				WithContext("package", pkg.Name)
		}

		baseURL = strings.TrimSuffix(resolved, "/") + "/"
	}

	url := baseURL + pkg.LocationHref
	dest := filepath.Join(destDir, filepath.Base(pkg.LocationHref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeNetwork, "failed to download package").
			WithOperation("downloadRPM").
			WithContext("package", pkg.Name)
	}

	defer func() { _ = resp.Body.Close() }()

	if err := httpclient.CheckStatus(resp, url); err != nil {
		return "", err
	}

	f, err := os.Create(dest) // #nosec G304
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(f, io.LimitReader(resp.Body, 512<<20)); err != nil {
		_ = f.Close()
		_ = os.Remove(dest)

		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write package file").
			WithOperation("downloadRPM").
			WithContext("package", pkg.Name)
	}

	if err := f.Close(); err != nil {
		return "", err
	}

	if pkg.SHA256 != "" {
		if ok, _ := fileMatchesSHA256(dest, pkg.SHA256); !ok {
			_ = os.Remove(dest)

			return "", errors.New(errors.ErrTypeValidation, "SHA256 mismatch").
				WithOperation("downloadRPM").
				WithContext("package", pkg.Name)
		}
	}

	logger.Debug("dnfcache: downloaded RPM", "package", pkg.Name, "dest", dest)

	return dest, nil
}

// rpmInstall installs the given .rpm files via "rpm --install --nodeps".
// --nodeps is used because we have already resolved the dependency closure.
func rpmInstall(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	args := append([]string{"--install", "--nodeps"}, paths...)

	cmd := exec.CommandContext(ctx, "rpm", args...) // #nosec G204
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("dnfcache: installing RPM packages", "count", len(paths))

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
		logger.Warn("dnfcache: unresolvable dependency", "package", name)
	}
}
