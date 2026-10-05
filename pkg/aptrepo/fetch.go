package aptrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// errNoPackagesVariant marks a (component, arch) pair the Release manifest
// does not list at all. apt treats this as "nothing to fetch" rather than a
// failure, so updateSource tolerates it; every other error is a real failure.
var errNoPackagesVariant = stderrors.New("no Packages variant found")

// indexLocks serialises work per destination file. Duplicate deb lines for
// the same (url, suite, component, arch) otherwise download and write the
// same list file concurrently.
var indexLocks sync.Map // destPath -> *sync.Mutex

func lockIndexPath(destPath string) (unlock func()) {
	m, _ := indexLocks.LoadOrStore(destPath, &sync.Mutex{})

	mu, ok := m.(*sync.Mutex)
	if !ok {
		return func() {}
	}

	mu.Lock()

	return mu.Unlock
}

// fetchComponentIndex downloads the Packages index for a component+arch combination
// into listsDir. It tries compression formats in size order: .xz, .gz, .bz2,
// uncompressed. A transport failure on one variant falls through to the next;
// if none succeeds the last transport error is returned (not a generic
// "no variant" message) so callers see the real cause.
func fetchComponentIndex(
	ctx context.Context, listsDir string, src *aptcache.SourceEntry, comp, arch string, rel *Release,
) error {
	candidates := []string{
		comp + "/binary-" + arch + "/Packages.xz",
		comp + "/binary-" + arch + "/Packages.gz",
		comp + "/binary-" + arch + "/Packages.bz2",
		comp + "/binary-" + arch + "/Packages",
	}

	var lastErr error

	for _, relPath := range candidates {
		entry, ok := rel.SHA256[relPath]
		if !ok {
			continue
		}

		stop, err := fetchIndexVariant(ctx, listsDir, src, relPath, entry)
		if err == nil {
			return nil
		}

		if stop {
			return err
		}

		lastErr = err
	}

	if lastErr != nil {
		return lastErr
	}

	return errors.Wrap(errNoPackagesVariant, errors.ErrTypeValidation, "no Packages variant found").
		WithOperation("fetchComponentIndex").
		WithContext("component", comp).
		WithContext("arch", arch)
}

// fetchIndexVariant fetches, verifies and atomically stores one Packages
// variant. stop reports that the error is definitive (integrity or I/O) and
// no other compression variant should be attempted; a plain transport error
// leaves stop false so the caller may try the next variant.
func fetchIndexVariant(
	ctx context.Context, listsDir string, src *aptcache.SourceEntry, relPath string, entry hashEntry,
) (stop bool, err error) {
	destPath := filepath.Join(listsDir, encodeListFilename(src.URL, src.Suite, relPath))

	unlock := lockIndexPath(destPath)
	defer unlock()

	// Warm-cache fast path: if the lists dir already holds a file for this
	// (src, suite, relPath) whose size and SHA256 match the freshly-verified
	// Release entry, skip the download entirely. This also makes duplicate
	// sources queued behind the per-path lock a no-op.
	if fileMatches(destPath, entry.Size, entry.Hash) {
		return true, nil
	}

	url := strings.TrimRight(src.URL, "/") + "/dists/" + src.Suite + "/" + relPath

	data, err := httpFetch(ctx, url)
	if err != nil {
		return false, err
	}

	if int64(len(data)) != entry.Size {
		return true, errors.New(errors.ErrTypeValidation, "size mismatch").
			WithOperation("fetchComponentIndex").
			WithContext("url", url).
			WithContext("got", len(data)).
			WithContext("expected", entry.Size)
	}

	sum := sha256.Sum256(data)

	got := hex.EncodeToString(sum[:])
	if got != entry.Hash {
		return true, errors.New(errors.ErrTypeValidation, "SHA256 mismatch").
			WithOperation("fetchComponentIndex").
			WithContext("url", url).
			WithContext("got", got).
			WithContext("expected", entry.Hash)
	}

	// Atomic temp-file + rename: a crash or a concurrent aptcache.Load never
	// observes a truncated Packages file.
	if err := httpclient.AtomicWrite(destPath, func(w io.Writer) error {
		_, werr := w.Write(data)

		return werr
	}); err != nil {
		return true, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write Packages index").
			WithOperation("fetchComponentIndex").
			WithContext("path", destPath)
	}

	return true, nil
}

// fileMatches reports whether destPath exists on disk with the expected size
// and SHA256. It's the warm-cache shortcut that avoids re-downloading
// unchanged Packages indexes. The file is hashed with a streaming reader so
// large indexes are never held in memory. Returns false on any stat / read /
// hash mismatch — the caller then falls through to the normal HTTP fetch path.
func fileMatches(destPath string, expectedSize int64, expectedHash string) bool {
	fi, err := os.Stat(destPath)
	if err != nil || fi.Size() != expectedSize {
		return false
	}

	f, err := os.Open(destPath) //nolint:gosec
	if err != nil {
		return false
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	return hex.EncodeToString(h.Sum(nil)) == expectedHash
}
