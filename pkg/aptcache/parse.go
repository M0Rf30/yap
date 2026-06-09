// parse.go: on-disk index loading and deb822 Packages/status parsing.

package aptcache

import (
	"bufio"
	"compress/bzip2"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	lz4 "github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// loadFromDisk reads all apt list files and the dpkg status file.
func loadFromDisk() *Cache {
	c := &Cache{
		entries:    make(map[string]*PackageInfo),
		providers:  make(map[string][]string),
		byBareName: make(map[string][]string),
	}

	// Load source schemes first (for BaseURL resolution)
	sources := loadSourceSchemes()

	// 1. Parse apt package index files from /var/lib/apt/lists/
	// Non-fatal: apt lists may not exist (e.g. non-Debian host).
	_ = c.loadAptLists(aptListsDir, sources)

	// 2. Overlay dpkg status (installed packages) — sets Installed flag and
	//    fills in any fields missing from the apt index.
	// Non-fatal: may not exist on non-Debian hosts.
	_ = c.loadDpkgStatus(dpkgStatusFile)

	return c
}

// loadAptLists scans dir for *_Packages files in all compression variants
// that apt may write: uncompressed, .gz, .bz2, .xz, .lz4, .zst.
// sources is a map from encoded hostpath to sourceInfo, used to resolve BaseURL.
//
// Performance: each Packages.* file is parsed concurrently into a private
// per-file Cache. The dominant cost is xz/zstd decompression + line
// scanning (~3-5s per file on a typical Ubuntu noble install); doing them
// in parallel collapses 16 sequential files into a single core-bound
// round, dropping load time from ~55s to ~10s on a 4-core host.
//
// Concurrency cap is min(GOMAXPROCS, 8) — diminishing returns past that
// because the work is CPU-bound and bigger pools just thrash the
// scheduler.
func (c *Cache) loadAptLists(dir string, sources map[string]sourceInfo) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	type job struct {
		path    string
		baseURL string
	}

	jobs := make([]job, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isPackagesIndexName(name) {
			continue
		}

		jobs = append(jobs, job{
			path:    filepath.Join(dir, name),
			baseURL: deriveBaseURL(name, sources),
		})
	}

	if len(jobs) == 0 {
		return nil
	}

	concurrency := min(runtime.GOMAXPROCS(0), 8, len(jobs))

	// Each worker parses into a thread-local Cache (no shared lock during
	// parse), then we merge sequentially at the end. mergeFrom is a plain
	// map-walk — much cheaper than per-stanza mu.Lock.
	partials := make([]*Cache, len(jobs))

	jobCh := make(chan int, len(jobs))
	for i := range jobs {
		jobCh <- i
	}

	close(jobCh)

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for idx := range jobCh {
				local := &Cache{
					entries:    make(map[string]*PackageInfo),
					providers:  make(map[string][]string),
					byBareName: make(map[string][]string),
				}
				// Skip unreadable/corrupt index files — apt itself is tolerant.
				_ = local.parseFile(jobs[idx].path, false, jobs[idx].baseURL)

				partials[idx] = local
			}
		}()
	}

	wg.Wait()

	for _, p := range partials {
		if p == nil {
			continue
		}

		c.mergeFrom(p)
	}

	return nil
}

// isPackagesIndexName reports whether name is one of the apt-emitted
// Packages index variants (uncompressed or one of the compressions apt
// supports).
func isPackagesIndexName(name string) bool {
	for _, suffix := range []string{
		"_Packages", "_Packages.gz", "_Packages.bz2",
		"_Packages.xz", "_Packages.lz4", "_Packages.zst",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

// mergeFrom folds a worker-local Cache produced by loadAptLists into c.
// Last-writer-wins on per-field merge into existing entries; providers
// are append-merged. Called under c.mu held by the orchestrator goroutine.
func (c *Cache) mergeFrom(other *Cache) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, info := range other.entries {
		existing, ok := c.entries[name]
		if !ok {
			// Copy the pointer; partial cache won't be touched after
			// this point.
			c.entries[name] = info
			// Also merge the secondary index entry for this key.
			if keys, ok := other.byBareName[info.Name]; ok {
				c.byBareName[info.Name] = append(c.byBareName[info.Name], keys...)
			}

			continue
		}

		mergeEntryFields(existing, info)
	}

	for virt, providers := range other.providers {
		c.providers[virt] = append(c.providers[virt], providers...)
	}
}

// mergeEntryFields applies a field-level merge of info into existing.
//
// When both entries carry a Version, artifact fields are taken from the
// higher-versioned side per Debian version comparison (ties favor info to
// fill in fields it may have, e.g. SHA256). Otherwise non-empty values from
// info win.
func mergeEntryFields(existing, info *PackageInfo) {
	if info.Architecture != "" {
		existing.Architecture = info.Architecture
	}

	if info.MultiArch != "" {
		existing.MultiArch = info.MultiArch
	}

	if info.Essential {
		existing.Essential = true
	}

	if info.HasCandidate {
		existing.HasCandidate = true
	}

	if !infoArtifactsWin(existing, info) {
		return
	}

	copyInfoArtifacts(existing, info)
}

// infoArtifactsWin decides whether info should overwrite existing's artifact
// fields. The chosen side is the one with the higher Debian version; ties and
// empty-Version edges fall back to "info wins if it carries any artifact data".
func infoArtifactsWin(existing, info *PackageInfo) bool {
	switch {
	case existing.Version == "" && info.Version == "":
		return info.Filename != "" || info.SHA256 != "" || info.Size > 0 ||
			len(info.Depends) > 0 || len(info.PreDepends) > 0
	case existing.Version == "":
		return true
	case info.Version == "":
		return false
	default:
		return CompareDebVersion(info.Version, existing.Version) >= 0
	}
}

// copyInfoArtifacts overwrites existing's artifact fields with non-empty
// values from info. BaseURL only follows Filename for non-"all" architectures,
// to avoid pointing an _all.deb at a foreign-arch mirror (ports.ubuntu.com).
func copyInfoArtifacts(existing, info *PackageInfo) {
	if info.Version != "" {
		existing.Version = info.Version
	}

	if info.Filename != "" {
		existing.Filename = info.Filename

		if info.BaseURL != "" && info.Architecture != "" && info.Architecture != archAll {
			existing.BaseURL = info.BaseURL
		}
	}

	if info.SHA256 != "" {
		existing.SHA256 = info.SHA256
	}

	if info.Size > 0 {
		existing.Size = info.Size
	}

	if len(info.Depends) > 0 {
		existing.Depends = info.Depends
	}

	if len(info.PreDepends) > 0 {
		existing.PreDepends = info.PreDepends
	}
}

// deriveBaseURL extracts the base URL from an apt list filename.
// Filename format: <encoded-hostpath>_dists_<suite>_<component>_binary-<arch>_Packages[.ext]
// Returns the full URL from sources map, or empty string if not found.
func deriveBaseURL(filename string, sources map[string]sourceInfo) string {
	// Strip compression suffix
	name := filename
	for _, ext := range []string{".gz", ".bz2", ".xz", ".lz4", ".zst"} {
		name = strings.TrimSuffix(name, ext)
	}

	// Find _dists_ separator
	prefix, _, found := strings.Cut(name, "_dists_")
	if !found {
		return ""
	}

	// Look up in sources map
	if info, ok := sources[prefix]; ok {
		return info.fullURL
	}

	return ""
}

// loadDpkgStatus parses the dpkg status database and marks packages as installed.
func (c *Cache) loadDpkgStatus(path string) error {
	return c.parseFile(path, true, "")
}

// parseFile opens a deb822 file (plain or gzip-compressed) and merges its
// stanzas into the cache. When dpkgStatus is true the "Status" field is
// checked and the Installed flag is set accordingly. baseURL is the repo
// base URL for apt index files (empty for dpkg status).
func (c *Cache) parseFile(path string, dpkgStatus bool, baseURL string) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open apt list file").
			WithContext("path", path).
			WithOperation("parseFile")
	}

	defer func() { _ = f.Close() }()

	var r io.Reader = f

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "failed to create gzip reader").
				WithContext("path", path).
				WithOperation("parseFile")
		}

		defer func() { _ = gz.Close() }()

		r = gz
	case strings.HasSuffix(path, ".bz2"):
		r = bzip2.NewReader(f)
	case strings.HasSuffix(path, ".xz"):
		// bufio.NewReader wraps the file in a buffered reader; ulikunitz/xz
		// reads in small chunks and without buffering this causes excessive
		// syscall overhead — 5-8x slower than buffered I/O.
		xzr, err := xz.NewReader(bufio.NewReader(f))
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "failed to create xz reader").
				WithContext("path", path).
				WithOperation("parseFile")
		}

		r = xzr
	case strings.HasSuffix(path, ".lz4"):
		// bufio.NewReader wraps the file in a buffered reader; pierrec/lz4
		// reads 4-byte block headers via io.ReadFull — without buffering this
		// causes one syscall per header, same issue as ulikunitz/xz above.
		r = lz4.NewReader(bufio.NewReader(f))
	case strings.HasSuffix(path, ".zst"):
		zr, err := zstd.NewReader(bufio.NewReader(f))
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "failed to create zstd reader").
				WithContext("path", path).
				WithOperation("parseFile")
		}

		defer zr.Close()

		r = zr
	}

	return c.parseDeb822(r, path, dpkgStatus, baseURL)
}

// stanza holds the fields extracted from a single deb822 stanza.
type stanza struct {
	pkgName    string
	arch       string
	multiArch  string
	provides   string
	depends    string
	preDepends string
	essential  bool
	installed  bool
	hasVersion bool
	version    string
	filename   string
	sha256     string
	size       int64
	baseURL    string
}

// parseDeb822 reads deb822 stanzas from r and merges them into the cache.
//
// The deb822 format is a sequence of stanzas separated by blank lines.
// Each stanza is a set of "Field: value" lines; continuation lines start
// with a space or tab.  We only extract the fields we care about.
// path is the source file path for error context.
// baseURL is the repo base URL for apt index files (empty for dpkg status).
//
//nolint:unparam // path kept for error context / API symmetry
func (c *Cache) parseDeb822(r io.Reader, path string, dpkgStatus bool, baseURL string) error {
	return deb822.Parse(r, func(stanzaMap deb822.Stanza) error {
		cur := stanza{baseURL: baseURL}

		// Apply each field from the parsed stanza
		for field, value := range stanzaMap {
			applyField(&cur, field, value, dpkgStatus)
		}

		// Flush the completed stanza into the cache
		c.flushStanza(&cur, dpkgStatus)

		return nil
	})
}

// applyField sets the appropriate stanza field from a parsed deb822 field line.
// Dispatches to applyCommonField or applyStatusField based on field type.
func applyField(s *stanza, field, value string, dpkgStatus bool) {
	switch field {
	case "Package":
		s.pkgName = value
	case "Version":
		s.hasVersion = value != ""
		s.version = value
	case "Architecture":
		s.arch = value
	case "Essential":
		s.essential = strings.EqualFold(value, "yes")
	case "Multi-Arch":
		s.multiArch = value
	case "Provides":
		s.provides = value
	case "Depends":
		s.depends = value
	case "Pre-Depends":
		s.preDepends = value
	case "Filename":
		s.filename = value
	case "Status":
		applyStatusField(s, value, dpkgStatus)
	case "SHA256", "Size":
		applyIndexField(s, field, value, dpkgStatus)
	}
}

// applyStatusField handles the Status field from dpkg status database.
func applyStatusField(s *stanza, value string, dpkgStatus bool) {
	if dpkgStatus {
		s.installed = value == "install ok installed"
	}
}

// applyIndexField handles apt index-only fields (SHA256, Size).
func applyIndexField(s *stanza, field, value string, dpkgStatus bool) {
	if dpkgStatus {
		return // only in apt index, not dpkg/status
	}

	switch field {
	case "SHA256":
		s.sha256 = value
	case "Size":
		n, _ := strconv.ParseInt(value, 10, 64)
		s.size = n
	}
}

// flushStanza merges a completed stanza into the cache using an
// architecture-qualified key (name:arch) to prevent arm64 entries from
// overwriting amd64 entries when both repositories are loaded.
// Delegates field merging to mergePackageInfo helper.
func (c *Cache) flushStanza(s *stanza, dpkgStatus bool) {
	if s.pkgName == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := entryKey(s.pkgName, s.arch)

	existing, ok := c.entries[key]
	if !ok {
		existing = &PackageInfo{}
		c.entries[key] = existing
	}

	mergePackageInfo(existing, s, dpkgStatus)
	c.flushProvides(s.pkgName, s.provides)
	c.addToBareName(s.pkgName, key)
}

// mergePackageInfo merges fields from a parsed stanza into an existing PackageInfo.
//
// For apt-index stanzas, when the existing entry already has a candidate, the
// incoming stanza overwrites artifact fields only when its Version is strictly
// greater per Debian version comparison. This implements "newest version wins"
// across multiple repositories indexing the same name:arch (matches apt's
// default behavior in the absence of explicit pinning).
func mergePackageInfo(existing *PackageInfo, s *stanza, dpkgStatus bool) {
	existing.Name = s.pkgName

	if s.arch != "" {
		existing.Architecture = s.arch
	}

	if s.essential {
		existing.Essential = true
	}

	if s.multiArch != "" {
		existing.MultiArch = s.multiArch
	}

	if dpkgStatus && s.installed {
		existing.Installed = true
	}

	if !s.hasVersion {
		return // pure virtual / dpkg status stanza, no artifact data
	}

	existing.HasCandidate = true

	if existing.Version != "" && CompareDebVersion(s.version, existing.Version) <= 0 {
		return // existing wins on version
	}

	copyStanzaArtifacts(existing, s)
}

// copyStanzaArtifacts overwrites the artifact fields of existing with values
// from a stanza. Used when the stanza wins on Debian version comparison.
func copyStanzaArtifacts(existing *PackageInfo, s *stanza) {
	existing.Version = s.version

	if s.filename != "" {
		existing.Filename = s.filename
	}

	if s.sha256 != "" {
		existing.SHA256 = s.sha256
	}

	if s.size > 0 {
		existing.Size = s.size
	}

	if s.baseURL != "" {
		existing.BaseURL = s.baseURL
	}

	if s.depends != "" {
		existing.Depends = parseDependsField(s.depends)
	}

	if s.preDepends != "" {
		existing.PreDepends = parseDependsField(s.preDepends)
	}
}

// parseDependsField parses a Depends or Pre-Depends field value and returns
// a list of package names. It handles:
//   - Comma-separated dependencies
//   - Alternative dependencies (foo | bar) — takes the first option
//   - Version constraints (>= 1.0) — stripped
//   - Architecture qualifiers (:any) — stripped
func parseDependsField(value string) []string {
	if value == "" {
		return nil
	}

	out := make([]string, 0, 4) // typical dep count

	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)

		// Alternative deps "foo | bar" — take the first.
		if i := strings.Index(p, "|"); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}

		// Strip version constraint "(>= 1.0)".
		if i := strings.Index(p, "("); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}

		// Strip arch qualifier ":any".
		if i := strings.Index(p, ":"); i >= 0 {
			p = p[:i]
		}

		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// flushProvides populates the reverse-provides index from a Provides field value.
// Must be called with c.mu already held.
func (c *Cache) flushProvides(pkgName, provides string) {
	if provides == "" {
		return
	}

	for prov := range strings.SplitSeq(provides, ",") {
		// Strip version constraint: "foo (= 1.0)" → "foo"
		vname, _, _ := strings.Cut(strings.TrimSpace(prov), " (")
		vname = strings.TrimSpace(vname)

		if vname == "" {
			continue
		}

		c.providers[vname] = append(c.providers[vname], pkgName)
	}
}
