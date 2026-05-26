package dnfcache

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	apperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	dnfCacheDir = "/var/cache/dnf"
	yumRepoDir  = "/etc/yum.repos.d"
)

// RepoEntry holds a single enabled repository parsed from a .repo file.
type RepoEntry struct {
	ID         string
	BaseURL    string // first baseurl= line
	MirrorList string // mirrorlist= URL (used when BaseURL is empty)
	Enabled    bool
}

// parseRepoFiles reads all *.repo files from /etc/yum.repos.d and returns
// the list of enabled repositories.
func parseRepoFiles() []RepoEntry {
	entries, err := os.ReadDir(yumRepoDir)
	if err != nil {
		return nil
	}

	var repos []RepoEntry

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".repo") {
			continue
		}

		path := filepath.Join(yumRepoDir, e.Name())

		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			continue
		}

		repos = append(repos, ParseRepoFileContent(string(data))...)
	}

	return repos
}

// ParseRepoFileContent parses the INI-style content of a .repo file.
func ParseRepoFileContent(content string) []RepoEntry {
	var repos []RepoEntry

	var cur RepoEntry

	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header [repoid]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if cur.ID != "" {
				repos = append(repos, cur)
			}

			cur = RepoEntry{
				ID:      line[1 : len(line)-1],
				Enabled: true, // default enabled
			}

			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "baseurl":
			if cur.BaseURL == "" {
				// Take the first URL (may be space/newline separated list).
				cur.BaseURL = strings.Fields(val)[0]
			}
		case "mirrorlist", "metalink":
			if cur.MirrorList == "" {
				cur.MirrorList = strings.Fields(val)[0]
			}
		case "enabled":
			cur.Enabled = val != "0"
		}
	}

	if cur.ID != "" {
		repos = append(repos, cur)
	}

	return repos
}

// ---- repomd.xml XML structs ----

type repoMD struct {
	XMLName xml.Name     `xml:"repomd"`
	Data    []repoMDData `xml:"data"`
}

type repoMDData struct {
	Type     string         `xml:"type,attr"`
	Location repoMDLocation `xml:"location"`
	Checksum repoMDChecksum `xml:"checksum"`
}

type repoMDLocation struct {
	Href string `xml:"href,attr"`
}

type repoMDChecksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// ---- primary.xml XML structs ----

type primaryPackage struct {
	Type     string          `xml:"type,attr"`
	Name     string          `xml:"name"`
	Arch     string          `xml:"arch"`
	Version  primaryVersion  `xml:"version"`
	Checksum primaryChecksum `xml:"checksum"`
	Size     primarySize     `xml:"size"`
	Location primaryLocation `xml:"location"`
	Format   primaryFormat   `xml:"format"`
}

type primaryVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type primaryChecksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type primarySize struct {
	Package int64 `xml:"package,attr"`
}

type primaryLocation struct {
	Href string `xml:"href,attr"`
}

type primaryFormat struct {
	Requires []primaryEntry `xml:"requires>entry"`
	Provides []primaryEntry `xml:"provides>entry"`
	// Files are file paths owned by the package and embedded in primary.xml
	// by createrepo_c's "primary files" subset (mostly /etc/*, /usr/bin/*,
	// /usr/sbin/*, /bin/*, /sbin/* and a few sendmail-style exceptions).
	// They are indexed as virtual providers so file-based Requires such as
	// "/usr/bin/perl" resolve to the owning package the way real dnf does.
	Files []string `xml:"file"`
}

type primaryEntry struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr"`
	Ver   string `xml:"ver,attr"`
}

// fetchAllRepos fetches repomd.xml + primary.xml.gz for all enabled repos
// and writes them to the DNF cache directory.
func fetchAllRepos(ctx context.Context) error {
	repos := parseRepoFiles()

	type result struct {
		id  string
		err error
	}

	// 8 workers comfortably covers the typical repo set (baseos, appstream,
	// extras, crb, epel, plus a few vendor repos) without overwhelming any
	// single mirror — sharedTransport already caps per-host connections.
	concurrency := min(min(runtime.GOMAXPROCS(0)*2, 8), len(repos))
	if concurrency == 0 {
		logger.Info("dnfcache: no enabled repos found")

		return nil
	}

	enabledCount := 0
	jobCh := make(chan RepoEntry, len(repos))

	for _, r := range repos {
		if r.Enabled && (r.BaseURL != "" || r.MirrorList != "") {
			jobCh <- r

			enabledCount++
		}
	}

	close(jobCh)

	logger.Info("dnfcache: fetching repos",
		"total", len(repos),
		"enabled", enabledCount,
		"workers", concurrency)

	resCh := make(chan result, len(repos))

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for repo := range jobCh {
				err := fetchRepo(ctx, repo)
				resCh <- result{id: repo.ID, err: err}
			}
		}()
	}

	wg.Wait()
	close(resCh)

	var firstErr error

	for res := range resCh {
		if res.err != nil {
			logger.Warn("dnfcache: failed to fetch repo",
				"repo", res.id,
				"error", res.err)

			// HTTP 4xx errors (auth-gated, not-found) are non-fatal: the
			// existing on-disk cache for that repo is still usable. Only
			// propagate errors that indicate a systemic problem (network
			// failure, disk full, etc.).
			if firstErr == nil && !isNonFatalRepoError(res.err) {
				firstErr = res.err
			}
		}
	}

	return firstErr
}

// fetchModules downloads modules.yaml when present. Always non-fatal: a
// missing or unfetchable modules index just disables module-stream
// filtering for that repo.
func fetchModules(ctx context.Context, repo RepoEntry, baseURL, repoCache, href, checksum string) {
	if href == "" {
		return
	}

	modulesURL := baseURL + "/" + strings.TrimPrefix(href, "/")
	modulesDest := filepath.Join(repoCache, filepath.Base(href))

	if err := downloadVerified(ctx, modulesURL, modulesDest, checksum); err != nil {
		logger.Warn("dnfcache: failed to fetch modules.yaml",
			"repo", repo.ID,
			"error", err)

		return
	}

	logger.Debug("dnfcache: modules index path", "repo", repo.ID, "file", modulesDest)
}

// resolveRepoBaseURL returns the concrete base URL for a repo, resolving
// mirrorlist/metalink if no baseurl was set.
func resolveRepoBaseURL(ctx context.Context, repo RepoEntry) (string, error) {
	baseURL := expandRepoVars(repo.BaseURL)

	if baseURL == "" && repo.MirrorList != "" {
		resolved, err := resolveMirrorList(ctx, expandRepoVars(repo.MirrorList))
		if err != nil {
			return "", apperrors.Wrap(err, apperrors.ErrTypeNetwork, "resolve mirrorlist").
				WithOperation("fetchRepo").
				WithContext("repo_id", repo.ID)
		}

		baseURL = resolved
	}

	if baseURL == "" {
		return "", apperrors.New(apperrors.ErrTypeConfiguration, "no baseurl or mirrorlist for repo").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	return strings.TrimSuffix(baseURL, "/"), nil
}

// repomdRefs holds the href + sha256 pair for one repodata entry.
type repomdRefs struct {
	primaryHref, primarySHA256 string
	modulesHref, modulesSHA256 string
}

// parseRepoMD downloads and decodes repomd.xml, returning the locations
// and checksums of the primary and modules data entries.
func parseRepoMD(ctx context.Context, repo RepoEntry, baseURL string) (repomdRefs, error) {
	var refs repomdRefs

	repomdData, err := fetchBytes(ctx, baseURL+"/repodata/repomd.xml")
	if err != nil {
		return refs, apperrors.Wrap(err, apperrors.ErrTypeNetwork, "fetch repomd.xml").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	var rmd repoMD
	if err := xml.Unmarshal(repomdData, &rmd); err != nil {
		return refs, apperrors.Wrap(err, apperrors.ErrTypeParser, "parse repomd.xml").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	for _, d := range rmd.Data {
		sha := ""
		if strings.EqualFold(d.Checksum.Type, "sha256") {
			sha = d.Checksum.Value
		}

		switch d.Type {
		case "primary":
			refs.primaryHref = d.Location.Href
			refs.primarySHA256 = sha
		case "modules":
			refs.modulesHref = d.Location.Href
			refs.modulesSHA256 = sha
		}
	}

	if refs.primaryHref == "" {
		return refs, apperrors.New(apperrors.ErrTypeParser, "no primary data in repomd.xml").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	return refs, nil
}

// fetchRepo fetches repomd.xml, primary.xml.gz, and (when present)
// modules.yaml for a single repo.
func fetchRepo(ctx context.Context, repo RepoEntry) error {
	baseURL, err := resolveRepoBaseURL(ctx, repo)
	if err != nil {
		return err
	}

	refs, err := parseRepoMD(ctx, repo, baseURL)
	if err != nil {
		return err
	}

	primaryURL := baseURL + "/" + strings.TrimPrefix(refs.primaryHref, "/")

	// Destination: /var/cache/dnf/<repoID>/repodata/<filename>
	repoCache := filepath.Join(dnfCacheDir, repo.ID, "repodata")
	if err := os.MkdirAll(repoCache, 0o755); err != nil {
		return err
	}

	// Persist the resolved baseURL so loadFromDisk can use it without
	// re-fetching the mirrorlist.
	baseurlFile := filepath.Join(dnfCacheDir, repo.ID, ".baseurl")
	if err := os.WriteFile(baseurlFile, []byte(baseURL), 0o644); err != nil { //nolint:gosec
		return err
	}

	destFile := filepath.Join(repoCache, filepath.Base(refs.primaryHref))

	if err := downloadVerified(ctx, primaryURL, destFile, refs.primarySHA256); err != nil {
		return apperrors.Wrap(err, apperrors.ErrTypeNetwork, "download primary.xml").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	if fi, statErr := os.Stat(destFile); statErr == nil {
		logger.Info("dnfcache: fetched repo",
			"repo", repo.ID,
			"url", baseURL,
			"bytes", fi.Size())
	} else {
		logger.Info("dnfcache: fetched repo", "repo", repo.ID, "url", baseURL)
	}

	logger.Debug("dnfcache: primary index path", "repo", repo.ID, "file", destFile)

	fetchModules(ctx, repo, baseURL, repoCache, refs.modulesHref, refs.modulesSHA256)

	return nil
}

// resolveMirrorList fetches a mirrorlist or metalink URL and returns the
// first usable base URL from the response.
//
// Plain mirrorlist: one URL per line, '#' lines are comments.
// Metalink XML: extracts the first https:// URL from <url> elements,
// strips the trailing /repodata/repomd.xml path to get the repo base URL.
//
// If the request returns HTTP 404 and the URL contains a dotted version
// (e.g. "8.10"), the fetch is retried with the major-only version ("8").
// Some Rocky Linux repos (e.g. Devel) only register major-version entries
// in the mirror manager.
func resolveMirrorList(ctx context.Context, mirrorListURL string) (string, error) {
	data, err := fetchBytes(ctx, mirrorListURL)
	if err != nil {
		// Retry with major-only releasever on 404 (e.g. Devel-8.10 → Devel-8).
		data, err = retryMajorVersion(ctx, mirrorListURL, err)
		if err != nil {
			return "", err
		}
	}

	body := strings.TrimSpace(string(data))

	// Metalink XML response.
	if strings.HasPrefix(body, "<") || strings.Contains(body[:min(len(body), 100)], "<?xml") {
		u, err := parseMedalinkURL(body, mirrorListURL)

		return normalizeURL(u), err
	}

	// Plain mirrorlist: first non-comment, non-empty line.
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		return normalizeURL(line), nil
	}

	return "", apperrors.New(apperrors.ErrTypeNetwork, "no usable mirror in mirrorlist").
		WithOperation("resolveMirrorList").
		WithContext("url", mirrorListURL)
}

// retryMajorVersion retries a mirrorlist fetch replacing "X.Y" releasever
// with "X" when the original request failed with HTTP 404.
func retryMajorVersion(ctx context.Context, mirrorListURL string, origErr error) ([]byte, error) {
	releasever := readReleasever()
	if !strings.Contains(releasever, ".") {
		return nil, origErr
	}

	major := strings.SplitN(releasever, ".", 2)[0]
	retryURL := strings.ReplaceAll(mirrorListURL, releasever, major)

	if retryURL == mirrorListURL {
		return nil, origErr
	}

	data, err := fetchBytes(ctx, retryURL)
	if err != nil {
		return nil, origErr // return original error for clarity
	}

	return data, nil
}

// parseMedalinkURL extracts the first https:// repo base URL from a
// Fedora/EPEL metalink XML response. The <url> elements point to
// repomd.xml files; we strip the trailing /repodata/repomd.xml.
func parseMedalinkURL(body, sourceURL string) (string, error) {
	// Simple scan — avoid a full XML decode for a hot path.
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)

		// Match <url ...>https://...</url> or bare https:// inside any tag.
		start := strings.Index(line, "https://")
		if start < 0 {
			continue
		}

		end := strings.IndexAny(line[start:], "<\" \t")
		if end < 0 {
			end = len(line[start:])
		}

		u := line[start : start+end]

		// Strip /repodata/repomd.xml suffix to get the base URL.
		if before, found := strings.CutSuffix(u, "/repodata/repomd.xml"); found {
			return before + "/", nil
		}

		// If no repomd.xml suffix, return as-is (plain mirror URL).
		if strings.HasSuffix(u, "/") {
			return u, nil
		}

		return u + "/", nil
	}

	return "", apperrors.New(apperrors.ErrTypeNetwork, "no usable mirror in metalink").
		WithOperation("parseMedalinkURL").
		WithContext("url", sourceURL)
}

// isNonFatalRepoError reports whether the error is an HTTP 4xx response,
// which is non-fatal for repo refresh: the existing on-disk cache stays
// usable (auth-gated, rate-limited, or temporarily unavailable repos).
func isNonFatalRepoError(err error) bool {
	var he *httpclient.HTTPStatusError
	if !errors.As(err, &he) {
		return false
	}

	return he.IsClientError()
}

// fetchBytes fetches a URL and returns its body as bytes.
func fetchBytes(ctx context.Context, url string) ([]byte, error) {
	return httpclient.FetchBytes(ctx, url, 64<<20) // 64 MiB cap
}

// downloadVerified downloads url to destFile, verifying SHA256 if provided.
// Skips download if destFile already exists with matching checksum.
func downloadVerified(ctx context.Context, url, destFile, expectedSHA256 string) error {
	// Skip if already cached with correct checksum.
	if expectedSHA256 != "" {
		if ok, _ := fileMatchesSHA256(destFile, expectedSHA256); ok {
			return nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := httpclient.CheckStatus(resp, url); err != nil {
		return err
	}

	h := sha256.New()

	if err := httpclient.AtomicWrite(destFile, func(w io.Writer) error {
		mw := io.MultiWriter(w, h)
		_, err := io.Copy(mw, io.LimitReader(resp.Body, 512<<20))

		return err
	}); err != nil {
		return err
	}

	if expectedSHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != expectedSHA256 {
			_ = os.Remove(destFile)

			return apperrors.New(apperrors.ErrTypePackaging, "SHA256 mismatch").
				WithOperation("downloadVerified").
				WithContext("url", url).
				WithContext("got", got).
				WithContext("want", expectedSHA256)
		}
	}

	return nil
}

// fileMatchesSHA256 returns true if path exists and its SHA256 matches expected.
func fileMatchesSHA256(path, expected string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return false, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	return hex.EncodeToString(h.Sum(nil)) == expected, nil
}

// loadFromDisk scans the DNF cache directory for primary.xml* files and
// parses them into the cache. Repos are parsed concurrently.
func (c *Cache) loadFromDisk() {
	// Load module-stream metadata FIRST so addPackage can filter
	// non-default-stream modular packages while parsing primary.xml.
	c.modules = loadModuleIndex()

	jobs := collectPrimaryFiles()

	if len(jobs) == 0 {
		logger.Debug("dnfcache: no primary.xml files to load")

		return
	}

	// Parse all primary.xml files concurrently. parsePrimaryFile acquires
	// c.mu internally per file, so concurrent calls are safe.
	concurrency := min(min(runtime.GOMAXPROCS(0), 4), len(jobs))

	jobCh := make(chan primaryFileJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}

	close(jobCh)

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for j := range jobCh {
				if err := c.parsePrimaryFile(j.path, j.burl); err != nil {
					logger.Warn("dnfcache: failed to parse primary index",
						"file", j.path,
						"error", err)
				}
			}
		}()
	}

	wg.Wait()

	c.mu.RLock()
	logger.Info("dnfcache: index loaded",
		"primary_files", len(jobs),
		"packages", len(c.packages),
		"capabilities", len(c.providers))
	c.mu.RUnlock()
}

// primaryFileJob holds a primary.xml file path and its repo base URL.
type primaryFileJob struct {
	path string
	burl string
}

// collectPrimaryFiles scans /etc/yum.repos.d and /var/cache/dnf to build
// the list of primary.xml files to parse.
func collectPrimaryFiles() []primaryFileJob {
	repos := parseRepoFiles()

	var jobs []primaryFileJob

	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}

		baseURL := strings.TrimSuffix(repo.BaseURL, "/")

		cacheDir := findRepoCacheDir(repo.ID)
		if cacheDir == "" {
			continue
		}

		if baseURL == "" {
			if b, err := os.ReadFile(filepath.Join(cacheDir, ".baseurl")); err == nil { //nolint:gosec
				baseURL = strings.TrimSpace(string(b))
			}
		}

		repoCache := filepath.Join(cacheDir, "repodata")

		entries, err := os.ReadDir(repoCache)
		if err != nil {
			continue
		}

		burl := baseURL + "/"
		if burl == "/" && repo.MirrorList != "" {
			burl = "mirrorlist:" + expandRepoVars(repo.MirrorList)
		}

		for _, e := range entries {
			if e.IsDir() || !isPrimaryIndex(e.Name()) {
				continue
			}

			jobs = append(jobs, primaryFileJob{
				path: filepath.Join(repoCache, e.Name()),
				burl: burl,
			})
		}
	}

	return jobs
}

// findRepoCacheDir returns the first directory under dnfCacheDir whose name
// starts with repoID (exact match or <repoID>-<hash> DNF convention).
func findRepoCacheDir(repoID string) string {
	// Exact match first (our own fetchRepo writes this).
	exact := filepath.Join(dnfCacheDir, repoID)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}

	// Glob for DNF-style <repoID>-<hash> dirs.
	entries, err := os.ReadDir(dnfCacheDir)
	if err != nil {
		return ""
	}

	prefix := repoID + "-"

	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return filepath.Join(dnfCacheDir, e.Name())
		}
	}

	return ""
}

// isPrimaryIndex reports whether name is a primary.xml variant.
func isPrimaryIndex(name string) bool {
	// Filenames are typically <sha256>-primary.xml.gz or primary.xml.gz
	base := name
	for _, ext := range []string{".gz", ".xz", ".zst"} {
		base = strings.TrimSuffix(base, ext)
	}

	return strings.HasSuffix(base, "primary.xml")
}

// parsePrimaryFile opens and parses a primary.xml file (possibly compressed).
func (c *Cache) parsePrimaryFile(path, baseURL string) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	var r io.Reader = f

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}

		defer func() { _ = gz.Close() }()

		r = gz

	case strings.HasSuffix(path, ".xz"):
		// bufio.NewReader wraps the file in a buffered reader; ulikunitz/xz
		// reads in small chunks and without buffering this causes excessive
		// syscall overhead — 5-8x slower than buffered I/O.
		xzr, err := xz.NewReader(bufio.NewReader(f))
		if err != nil {
			return err
		}

		r = xzr

	case strings.HasSuffix(path, ".zst"):
		zr, err := zstd.NewReader(bufio.NewReader(f))
		if err != nil {
			return err
		}

		defer zr.Close()

		r = zr
	}

	return c.parsePrimaryXML(r, baseURL)
}

// parsePrimaryXML decodes primary.xml from r and merges packages into the cache.
func (c *Cache) parsePrimaryXML(r io.Reader, baseURL string) error {
	decoder := xml.NewDecoder(r)

	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "package" {
			continue
		}

		var pkg primaryPackage
		if err := decoder.DecodeElement(&pkg, &se); err != nil {
			continue
		}

		if info := buildPackageInfo(&pkg, baseURL); info != nil {
			c.addPackage(info)
		}
	}

	return nil
}

// buildPackageInfo converts a parsed primaryPackage into a PackageInfo.
// Returns nil if the package should be skipped (e.g. source RPMs, empty name).
func buildPackageInfo(pkg *primaryPackage, baseURL string) *PackageInfo {
	if pkg.Name == "" || pkg.Arch == "" || pkg.Arch == "src" {
		return nil
	}

	requires := make([]string, 0, len(pkg.Format.Requires))

	for _, req := range pkg.Format.Requires {
		name := StripRPMConstraint(req.Name)
		// Skip rpmlib() deps — they describe rpm-format features, not packages.
		// Path-style deps (e.g. "/usr/bin/perl") are kept and resolved via the
		// file paths indexed from <file> entries in primary.xml.
		if name == "" || strings.HasPrefix(name, "rpmlib(") {
			continue
		}

		requires = append(requires, name)
	}

	provides := make([]string, 0, len(pkg.Format.Provides)+len(pkg.Format.Files))

	for _, prov := range pkg.Format.Provides {
		if prov.Name != "" {
			provides = append(provides, prov.Name)
		}
	}

	// Owned file paths act as virtual providers so file-based Requires
	// resolve to the package that owns the file. createrepo_c only embeds a
	// curated subset of paths in primary.xml; the full filelist lives in
	// filelists.xml and is intentionally not loaded here (size).
	for _, f := range pkg.Format.Files {
		if f != "" {
			provides = append(provides, f)
		}
	}

	info := &PackageInfo{
		Name:         pkg.Name,
		Arch:         pkg.Arch,
		Version:      pkg.Version.Ver,
		Release:      pkg.Version.Rel,
		Epoch:        pkg.Version.Epoch,
		LocationHref: pkg.Location.Href,
		Size:         pkg.Size.Package,
		BaseURL:      baseURL,
		Requires:     requires,
		Provides:     provides,
	}

	if strings.EqualFold(pkg.Checksum.Type, "sha256") {
		info.SHA256 = pkg.Checksum.Value
	}

	return info
}
