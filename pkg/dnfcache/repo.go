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
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	apperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// dnfCacheDir and yumRepoDir are package-level vars (not consts) so tests
// can redirect them to temp directories.
var (
	dnfCacheDir = "/var/cache/dnf"
	yumRepoDir  = "/etc/yum.repos.d"
)

// RepoEntry holds a single enabled repository parsed from a .repo file.
type RepoEntry struct {
	ID         string
	BaseURL    string   // first baseurl= value (kept for single-URL callers)
	BaseURLs   []string // all baseurl= values in listed order
	MirrorList string   // mirrorlist= / metalink= URL (used when no baseurl)
	Enabled    bool
}

// baseURLs returns every configured baseurl for the repo, tolerating
// entries constructed with only the legacy BaseURL field set.
func (r *RepoEntry) baseURLs() []string {
	if len(r.BaseURLs) > 0 {
		return r.BaseURLs
	}

	if r.BaseURL != "" {
		return []string{r.BaseURL}
	}

	return nil
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

		applyRepoLine(&cur, line)
	}

	if cur.ID != "" {
		repos = append(repos, cur)
	}

	return repos
}

// applyRepoLine folds one non-section line of a .repo file into cur:
// bare-URL baseurl continuation lines and key=value assignments.
func applyRepoLine(cur *RepoEntry, line string) {
	// Multi-line baseurl continuation: dnf's INI dialect allows extra
	// URLs on indented lines following a "baseurl=" line.
	if cur.ID != "" && isRepoURL(line) {
		cur.BaseURLs = append(cur.BaseURLs, line)
		if cur.BaseURL == "" {
			cur.BaseURL = line
		}

		return
	}

	key, val, ok := strings.Cut(line, "=")
	if !ok {
		return
	}

	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)

	switch key {
	case "baseurl":
		// Collect every URL (may be a space/newline separated list).
		urls := strings.Fields(val)

		cur.BaseURLs = append(cur.BaseURLs, urls...)
		if cur.BaseURL == "" && len(urls) > 0 {
			cur.BaseURL = urls[0]
		}
	case "mirrorlist", "metalink":
		if cur.MirrorList == "" {
			cur.MirrorList = strings.Fields(val)[0]
		}
	case "enabled":
		cur.Enabled = val != "0"
	}
}

// isRepoURL reports whether line looks like a bare repository URL
// (a baseurl continuation line in a .repo file).
func isRepoURL(line string) bool {
	return strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") ||
		strings.HasPrefix(line, "ftp://") || strings.HasPrefix(line, "file://")
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
	Requires   []primaryEntry `xml:"requires>entry"`
	Provides   []primaryEntry `xml:"provides>entry"`
	Recommends []primaryEntry `xml:"recommends>entry"`
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
		logger.Info(i18n.T("logger.dnfcache.info.no_enabled_repos_found"))

		return nil
	}

	enabledCount := 0
	jobCh := make(chan RepoEntry, len(repos))

	for _, r := range repos {
		if r.Enabled && (len(r.baseURLs()) > 0 || r.MirrorList != "") {
			jobCh <- r

			enabledCount++
		}
	}

	close(jobCh)

	logger.Info(i18n.T("logger.dnfcache.info.fetching_repos"), "total", len(repos),
		"enabled", enabledCount,
		"workers", concurrency)

	resCh := make(chan result, len(repos))

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for repo := range jobCh {
				err := fetchRepo(ctx, &repo)
				resCh <- result{id: repo.ID, err: err}
			}
		}()
	}

	wg.Wait()
	close(resCh)

	var firstErr error

	for res := range resCh {
		if res.err != nil {
			logger.Warn(i18n.T("logger.dnfcache.warn.failed_fetch_repo"), "repo", res.id,
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
func fetchModules(ctx context.Context, repo *RepoEntry, baseURL, repoCache, href, checksum string) {
	if href == "" {
		return
	}

	modulesURL := baseURL + "/" + strings.TrimPrefix(href, "/")
	modulesDest := filepath.Join(repoCache, filepath.Base(href))

	if err := downloadVerified(ctx, modulesURL, modulesDest, checksum); err != nil {
		logger.Warn(i18n.T("logger.dnfcache.warn.failed_fetch_modules_yaml"), "repo", repo.ID,
			"error", err)

		return
	}

	logger.Debug(i18n.T("logger.dnfcache.debug.modules_index_path"), "repo", repo.ID, "file", modulesDest)
}

// maxMirrors caps how many candidate mirrors are kept per repo. The first
// healthy one wins; trying more than a handful only delays the failure
// report when a repo is truly unreachable.
const maxMirrors = 5

// resolveRepoCandidates returns the ordered list of candidate base URLs
// for a repo: every baseurl= entry (vars expanded), or — when none is set
// — up to maxMirrors mirrors resolved from the mirrorlist/metalink URL.
func resolveRepoCandidates(ctx context.Context, repo *RepoEntry) ([]string, error) {
	var candidates []string

	for _, raw := range repo.baseURLs() {
		u := normalizeURL(expandRepoVars(raw))
		if u != "" {
			candidates = append(candidates, strings.TrimSuffix(u, "/"))
		}
	}

	if len(candidates) == 0 && repo.MirrorList != "" {
		mirrors, err := resolveMirrors(ctx, expandRepoVars(repo.MirrorList))
		if err != nil {
			return nil, apperrors.Wrap(err, apperrors.ErrTypeNetwork, "resolve mirrorlist").
				WithOperation("fetchRepo").
				WithContext("repo_id", repo.ID)
		}

		for _, m := range mirrors {
			candidates = append(candidates, strings.TrimSuffix(m, "/"))
		}
	}

	if len(candidates) == 0 {
		return nil, apperrors.New(apperrors.ErrTypeConfiguration, "no baseurl or mirrorlist for repo").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	if len(candidates) > maxMirrors {
		candidates = candidates[:maxMirrors]
	}

	return candidates, nil
}

// repomdRefs holds the href + sha256 pair for one repodata entry.
type repomdRefs struct {
	primaryHref, primarySHA256 string
	modulesHref, modulesSHA256 string
}

// parseRepoMD downloads and decodes repomd.xml, returning the locations
// and checksums of the primary and modules data entries.
//
// Warm-cache: when a cached repomd.xml already exists under
// /var/cache/dnf/<repoID>/repodata/, its mtime is used as
// If-Modified-Since. On HTTP 304 the cached file is parsed directly and
// no body is downloaded — the typical case when the repo's repomd hasn't
// moved. The cached file is refreshed (with the same content) on 200.
func parseRepoMD(ctx context.Context, repo *RepoEntry, baseURL string) (repomdRefs, error) {
	var refs repomdRefs

	repomdURL := baseURL + "/repodata/repomd.xml"
	cachedPath := filepath.Join(dnfCacheDir, repo.ID, "repodata", "repomd.xml")

	var ifModSince time.Time
	if fi, err := os.Stat(cachedPath); err == nil {
		ifModSince = fi.ModTime()
	}

	repomdData, notModified, err := httpclient.FetchBytesConditional(
		ctx, repomdURL, 64<<20, ifModSince,
	)
	if err != nil {
		return refs, apperrors.Wrap(err, apperrors.ErrTypeNetwork, "fetch repomd.xml").
			WithOperation("fetchRepo").
			WithContext("repo_id", repo.ID)
	}

	if notModified {
		// Server confirmed cache is current; use on-disk copy.
		cached, readErr := os.ReadFile(cachedPath) //nolint:gosec
		if readErr != nil {
			return refs, apperrors.Wrap(readErr, apperrors.ErrTypeFileSystem,
				"read cached repomd.xml").
				WithOperation("fetchRepo").
				WithContext("repo_id", repo.ID)
		}

		repomdData = cached
	} else {
		// Persist fresh repomd.xml so the next call can use If-Modified-Since.
		if mkErr := os.MkdirAll(filepath.Dir(cachedPath), 0o755); mkErr == nil {
			_ = os.WriteFile(cachedPath, repomdData, 0o644) //nolint:gosec
		}
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
// modules.yaml for a single repo, failing over across candidate mirrors:
// when one mirror is unreachable, stale (checksum mismatch), or missing
// the repo path (404), the next candidate is tried. repomd.xml and the
// files it references are always fetched from the SAME mirror so a
// mid-sync mirror cannot mix metadata generations.
func fetchRepo(ctx context.Context, repo *RepoEntry) error {
	candidates, err := resolveRepoCandidates(ctx, repo)
	if err != nil {
		return err
	}

	var lastErr error

	for i, baseURL := range candidates {
		err := fetchRepoFrom(ctx, repo, baseURL)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't burn the remaining mirrors when the caller is gone.
		if ctx.Err() != nil {
			break
		}

		if i < len(candidates)-1 {
			logger.Warn(i18n.T("logger.dnfcache.warn.mirror_failed_trying_next"),
				"repo", repo.ID,
				"mirror", baseURL,
				"remaining", len(candidates)-i-1,
				"error", err)
		}
	}

	return lastErr
}

// fetchRepoFrom fetches the full metadata set for repo from a single mirror.
func fetchRepoFrom(ctx context.Context, repo *RepoEntry, baseURL string) error {
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
		logger.Info(i18n.T("logger.dnfcache.info.fetched_repo"), "repo", repo.ID,
			"url", baseURL,
			"bytes", fi.Size())
	} else {
		logger.Info(i18n.T("logger.dnfcache.info.fetched_repo"), "repo", repo.ID, "url", baseURL)
	}

	logger.Debug(i18n.T("logger.dnfcache.debug.primary_index_path"), "repo", repo.ID, "file", destFile)

	fetchModules(ctx, repo, baseURL, repoCache, refs.modulesHref, refs.modulesSHA256)

	return nil
}

// resolveMirrors fetches a mirrorlist or metalink URL and returns the
// usable base URLs from the response, in server-preferred order, capped
// at maxMirrors.
//
// Plain mirrorlist: one URL per line, '#' lines are comments.
// Metalink XML: extracts http(s):// URLs from <url> elements, stripping
// the trailing /repodata/repomd.xml path to get the repo base URL.
//
// If the request returns HTTP 404 and the URL contains a dotted version
// (e.g. "8.10"), the fetch is retried with the major-only version ("8").
// Some Rocky Linux repos (e.g. Devel) only register major-version entries
// in the mirror manager.
func resolveMirrors(ctx context.Context, mirrorListURL string) ([]string, error) {
	data, err := fetchBytes(ctx, mirrorListURL)
	if err != nil {
		// Retry with major-only releasever on 404 (e.g. Devel-8.10 → Devel-8).
		data, err = retryMajorVersion(ctx, mirrorListURL, err)
		if err != nil {
			return nil, err
		}
	}

	body := strings.TrimSpace(string(data))

	// Metalink XML response.
	if strings.HasPrefix(body, "<") || strings.Contains(body[:min(len(body), 100)], "<?xml") {
		return parseMetalinkURLs(body, mirrorListURL)
	}

	// Plain mirrorlist: one URL per non-comment, non-empty line.
	var mirrors []string

	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		mirrors = append(mirrors, normalizeURL(line))

		if len(mirrors) == maxMirrors {
			break
		}
	}

	if len(mirrors) == 0 {
		return nil, apperrors.New(apperrors.ErrTypeNetwork, "no usable mirror in mirrorlist").
			WithOperation("resolveMirrors").
			WithContext("url", mirrorListURL)
	}

	return mirrors, nil
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

// parseMetalinkURLs extracts repo base URLs from a Fedora/EPEL metalink
// XML response, preserving the server's preference order. The <url>
// elements point to repomd.xml files; the trailing /repodata/repomd.xml
// is stripped. https:// mirrors are preferred; plain http:// mirrors are
// appended after them as a last resort. Returns at most maxMirrors URLs.
func parseMetalinkURLs(body, sourceURL string) ([]string, error) {
	var httpsMirrors, httpMirrors []string

	// Simple scan — avoid a full XML decode for a hot path.
	for line := range strings.SplitSeq(body, "\n") {
		u, secure := extractMirrorURL(strings.TrimSpace(line))
		if u == "" {
			continue
		}

		if secure {
			httpsMirrors = append(httpsMirrors, u)
		} else {
			httpMirrors = append(httpMirrors, u)
		}
	}

	mirrors := append(httpsMirrors, httpMirrors...) //nolint:gocritic
	if len(mirrors) == 0 {
		return nil, apperrors.New(apperrors.ErrTypeNetwork, "no usable mirror in metalink").
			WithOperation("parseMetalinkURLs").
			WithContext("url", sourceURL)
	}

	if len(mirrors) > maxMirrors {
		mirrors = mirrors[:maxMirrors]
	}

	for i, m := range mirrors {
		mirrors[i] = normalizeURL(m)
	}

	return mirrors, nil
}

// extractMirrorURL pulls an http(s) URL out of one metalink line and
// normalizes it to a base URL (strips /repodata/repomd.xml, ensures a
// trailing slash). Returns ("", false) when the line holds no usable URL.
func extractMirrorURL(line string) (string, bool) {
	secure := true

	start := strings.Index(line, "https://")
	if start < 0 {
		secure = false

		start = strings.Index(line, "http://")
		if start < 0 {
			return "", false
		}
	}

	end := strings.IndexAny(line[start:], "<\" \t")
	if end < 0 {
		end = len(line[start:])
	}

	u := line[start : start+end]

	// Strip /repodata/repomd.xml suffix to get the base URL.
	if before, found := strings.CutSuffix(u, "/repodata/repomd.xml"); found {
		return before + "/", secure
	}

	if strings.HasSuffix(u, "/") {
		return u, secure
	}

	return u + "/", secure
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
// Transient network failures are retried per the httpclient retry policy;
// a checksum mismatch is NOT retried here — it usually means the mirror is
// mid-sync, and the caller's mirror-failover loop moves to the next mirror.
func downloadVerified(ctx context.Context, url, destFile, expectedSHA256 string) error {
	// Skip if already cached with correct checksum.
	if expectedSHA256 != "" {
		if ok, _ := fileMatchesSHA256(destFile, expectedSHA256); ok {
			return nil
		}
	}

	return httpclient.WithRetry(ctx, url, func() error {
		return downloadVerifiedOnce(ctx, url, destFile, expectedSHA256)
	})
}

// downloadVerifiedOnce performs a single download + verify attempt.
func downloadVerifiedOnce(ctx context.Context, url, destFile, expectedSHA256 string) error {
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
		logger.Debug(i18n.T("logger.dnfcache.debug.no_primary_xml_files"))

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
					logger.Warn(i18n.T("logger.dnfcache.warn.failed_parse_primary_index"), "file", j.path,
						"error", err)
				}
			}
		}()
	}

	wg.Wait()

	c.mu.RLock()
	logger.Info(i18n.T("logger.dnfcache.info.index_loaded"), "primary_files", len(jobs),
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

		cacheDir := findRepoCacheDir(repo.ID)
		if cacheDir == "" {
			continue
		}

		// Prefer the mirror actually used at fetch time (persisted by
		// fetchRepoFrom) so package downloads hit a mirror known to carry
		// this metadata generation; fall back to the configured baseurl.
		baseURL := ""
		if b, err := os.ReadFile(filepath.Join(cacheDir, ".baseurl")); err == nil { //nolint:gosec
			baseURL = strings.TrimSpace(string(b))
		}

		if baseURL == "" {
			baseURL = strings.TrimSuffix(expandRepoVars(repo.BaseURL), "/")
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

	// Recommends are weak dependencies. dnf installs them by default; we mirror
	// that behaviour so build environments match what an rpmbuild-style install
	// would produce (e.g. redhat-rpm-config recommends gcc-plugin-annobin,
	// without which any package using -specs=redhat-annobin-cc1 fails to compile).
	recommends := make([]string, 0, len(pkg.Format.Recommends))

	for _, rec := range pkg.Format.Recommends {
		name := StripRPMConstraint(rec.Name)
		if name == "" || strings.HasPrefix(name, "rpmlib(") {
			continue
		}

		recommends = append(recommends, name)
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
		Recommends:   recommends,
	}

	if strings.EqualFold(pkg.Checksum.Type, "sha256") {
		info.SHA256 = pkg.Checksum.Value
	}

	return info
}
