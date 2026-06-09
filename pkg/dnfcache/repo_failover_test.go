//nolint:testpackage
package dnfcache

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// TestMain shrinks the httpclient retry backoff so transient-failure tests
// don't sleep through the production backoff schedule.
func TestMain(m *testing.M) {
	httpclient.SetRetryPolicy(3, time.Millisecond)
	os.Exit(m.Run())
}

// ---- ParseRepoFileContent: multiple baseurls ----

// TestParseRepoFileContentMultipleBaseURLs tests that every baseurl is
// collected: space-separated lists and dnf-style indented continuation lines.
func TestParseRepoFileContentMultipleBaseURLs(t *testing.T) {
	content := `[multi]
name=Multi-mirror repo
baseurl=https://mirror1.example.com/repo/ https://mirror2.example.com/repo/
        https://mirror3.example.com/repo/
enabled=1
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	repo := repos[0]
	if repo.BaseURL != "https://mirror1.example.com/repo/" {
		t.Errorf("unexpected first BaseURL: %q", repo.BaseURL)
	}

	want := []string{
		"https://mirror1.example.com/repo/",
		"https://mirror2.example.com/repo/",
		"https://mirror3.example.com/repo/",
	}

	if len(repo.BaseURLs) != len(want) {
		t.Fatalf("expected %d baseurls, got %d: %v", len(want), len(repo.BaseURLs), repo.BaseURLs)
	}

	for i, u := range want {
		if repo.BaseURLs[i] != u {
			t.Errorf("BaseURLs[%d] = %q, want %q", i, repo.BaseURLs[i], u)
		}
	}
}

// TestRepoEntryBaseURLsLegacyField tests that entries constructed with only
// the legacy single BaseURL field still resolve candidates.
func TestRepoEntryBaseURLsLegacyField(t *testing.T) {
	repo := RepoEntry{ID: "legacy", BaseURL: "https://only.example.com/repo/", Enabled: true}

	urls := repo.baseURLs()
	if len(urls) != 1 || urls[0] != "https://only.example.com/repo/" {
		t.Errorf("unexpected baseURLs: %v", urls)
	}
}

// ---- resolveRepoCandidates ----

// TestResolveRepoCandidatesFromBaseURLs tests baseurl expansion, slash
// trimming, and the maxMirrors cap.
func TestResolveRepoCandidatesFromBaseURLs(t *testing.T) {
	repo := RepoEntry{ID: "many", Enabled: true}
	for i := range maxMirrors + 2 {
		repo.BaseURLs = append(repo.BaseURLs,
			fmt.Sprintf("https://m%d.example.com/repo/", i))
	}

	got, err := resolveRepoCandidates(context.Background(), &repo)
	if err != nil {
		t.Fatalf("resolveRepoCandidates failed: %v", err)
	}

	if len(got) != maxMirrors {
		t.Fatalf("expected %d candidates, got %d", maxMirrors, len(got))
	}

	if got[0] != "https://m0.example.com/repo" {
		t.Errorf("expected trailing slash trimmed, got %q", got[0])
	}
}

// TestResolveRepoCandidatesNoConfig tests that a repo without baseurl or
// mirrorlist yields a configuration error.
func TestResolveRepoCandidatesNoConfig(t *testing.T) {
	_, err := resolveRepoCandidates(context.Background(), &RepoEntry{ID: "empty", Enabled: true})
	if err == nil {
		t.Fatal("expected error for repo without baseurl/mirrorlist")
	}
}

// TestResolveRepoCandidatesMirrorlist tests that mirrorlist resolution
// returns every usable mirror, in order.
func TestResolveRepoCandidatesMirrorlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# comment line\n\nhttps://a.example.com/repo/\nhttps://b.example.com/repo/\n"))
	}))
	defer srv.Close()

	repo := RepoEntry{ID: "ml", MirrorList: srv.URL, Enabled: true}

	got, err := resolveRepoCandidates(context.Background(), &repo)
	if err != nil {
		t.Fatalf("resolveRepoCandidates failed: %v", err)
	}

	if len(got) != 2 || got[0] != "https://a.example.com/repo" || got[1] != "https://b.example.com/repo" {
		t.Errorf("unexpected candidates: %v", got)
	}
}

// ---- resolveMirrors ----

// TestResolveMirrorsMetalink tests metalink XML resolution returning all
// mirrors in preference order.
func TestResolveMirrorsMetalink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="repomd.xml">
    <url protocol="https" preference="100">https://fast.example.com/repo/repodata/repomd.xml</url>
    <url protocol="https" preference="90">https://slow.example.com/repo/repodata/repomd.xml</url>
  </file>
</metalink>`))
	}))
	defer srv.Close()

	got, err := resolveMirrors(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("resolveMirrors failed: %v", err)
	}

	if len(got) != 2 || got[0] != "https://fast.example.com/repo/" || got[1] != "https://slow.example.com/repo/" {
		t.Errorf("unexpected mirrors: %v", got)
	}
}

// TestResolveMirrorsPlainListCapped tests that a long plain mirrorlist is
// capped at maxMirrors entries.
func TestResolveMirrorsPlainListCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for i := range 12 {
			_, _ = fmt.Fprintf(w, "https://m%d.example.com/repo/\n", i)
		}
	}))
	defer srv.Close()

	got, err := resolveMirrors(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("resolveMirrors failed: %v", err)
	}

	if len(got) != maxMirrors {
		t.Errorf("expected %d mirrors, got %d", maxMirrors, len(got))
	}
}

// ---- fetchRepo failover (end to end against httptest mirrors) ----

// repoMirrorFixture builds the repomd.xml + primary.xml.gz pair served by
// the fake mirrors.
type repoMirrorFixture struct {
	repomdXML  string
	primaryGz  []byte
	primarySHA string
}

func newRepoMirrorFixture(t *testing.T) repoMirrorFixture {
	t.Helper()

	var gzBuf []byte
	{
		var b []byte

		w := &writerBuf{buf: &b}
		gz := gzip.NewWriter(w)

		_, err := gz.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" packages="0">
</metadata>`))
		if err != nil {
			t.Fatalf("gzip write: %v", err)
		}

		if err := gz.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}

		gzBuf = b
	}

	sum := sha256.Sum256(gzBuf)
	sha := hex.EncodeToString(sum[:])

	repomd := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <checksum type="sha256">%s</checksum>
    <location href="repodata/primary.xml.gz"/>
  </data>
</repomd>`, sha)

	return repoMirrorFixture{repomdXML: repomd, primaryGz: gzBuf, primarySHA: sha}
}

// writerBuf is a minimal io.Writer over a byte-slice pointer.
type writerBuf struct{ buf *[]byte }

func (w *writerBuf) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)

	return len(p), nil
}

// withTempDNFCacheDir redirects the package-level DNF cache dir to a temp
// directory for the duration of the test.
func withTempDNFCacheDir(t *testing.T) string {
	t.Helper()

	orig := dnfCacheDir
	tmp := t.TempDir()
	dnfCacheDir = tmp

	t.Cleanup(func() { dnfCacheDir = orig })

	return tmp
}

// TestFetchRepoFailsOverOnDeadMirror tests that fetchRepo skips a mirror
// whose repo path is missing (404) and succeeds from the next one.
func TestFetchRepoFailsOverOnDeadMirror(t *testing.T) {
	cacheDir := withTempDNFCacheDir(t)
	fixture := newRepoMirrorFixture(t)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer dead.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repodata/repomd.xml":
			_, _ = w.Write([]byte(fixture.repomdXML))
		case "/repodata/primary.xml.gz":
			_, _ = w.Write(fixture.primaryGz)
		default:
			http.NotFound(w, r)
		}
	}))
	defer good.Close()

	repo := RepoEntry{
		ID:       "failover",
		BaseURLs: []string{dead.URL, good.URL},
		Enabled:  true,
	}

	if err := fetchRepo(context.Background(), &repo); err != nil {
		t.Fatalf("fetchRepo failed: %v", err)
	}

	// The persisted .baseurl must point at the mirror that actually worked.
	b, err := os.ReadFile(filepath.Join(cacheDir, "failover", ".baseurl"))
	if err != nil {
		t.Fatalf("read .baseurl: %v", err)
	}

	if string(b) != good.URL {
		t.Errorf(".baseurl = %q, want %q", b, good.URL)
	}

	// primary.xml.gz must be on disk and checksum-clean.
	primaryPath := filepath.Join(cacheDir, "failover", "repodata", "primary.xml.gz")
	if ok, _ := fileMatchesSHA256(primaryPath, fixture.primarySHA); !ok {
		t.Error("primary.xml.gz missing or checksum mismatch after failover")
	}
}

// TestFetchRepoFailsOverOnStaleMirror tests the mid-sync mirror case: the
// first mirror serves a valid repomd.xml but a corrupt primary payload
// (checksum mismatch), so the whole repo fetch must move to the next mirror.
func TestFetchRepoFailsOverOnStaleMirror(t *testing.T) {
	cacheDir := withTempDNFCacheDir(t)
	fixture := newRepoMirrorFixture(t)

	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repodata/repomd.xml":
			_, _ = w.Write([]byte(fixture.repomdXML))
		case "/repodata/primary.xml.gz":
			_, _ = w.Write([]byte("corrupt payload from a mid-sync mirror"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer stale.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repodata/repomd.xml":
			_, _ = w.Write([]byte(fixture.repomdXML))
		case "/repodata/primary.xml.gz":
			_, _ = w.Write(fixture.primaryGz)
		default:
			http.NotFound(w, r)
		}
	}))
	defer good.Close()

	repo := RepoEntry{
		ID:       "stale",
		BaseURLs: []string{stale.URL, good.URL},
		Enabled:  true,
	}

	if err := fetchRepo(context.Background(), &repo); err != nil {
		t.Fatalf("fetchRepo failed: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(cacheDir, "stale", ".baseurl"))
	if err != nil {
		t.Fatalf("read .baseurl: %v", err)
	}

	if string(b) != good.URL {
		t.Errorf(".baseurl = %q, want %q", b, good.URL)
	}

	primaryPath := filepath.Join(cacheDir, "stale", "repodata", "primary.xml.gz")
	if ok, _ := fileMatchesSHA256(primaryPath, fixture.primarySHA); !ok {
		t.Error("primary.xml.gz missing or checksum mismatch after failover")
	}
}

// TestFetchRepoAllMirrorsDead tests that fetchRepo reports an error when
// every candidate mirror fails.
func TestFetchRepoAllMirrorsDead(t *testing.T) {
	withTempDNFCacheDir(t)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer dead.Close()

	repo := RepoEntry{
		ID:       "alldead",
		BaseURLs: []string{dead.URL, dead.URL + "/other"},
		Enabled:  true,
	}

	if err := fetchRepo(context.Background(), &repo); err == nil {
		t.Fatal("expected error when all mirrors fail")
	}
}

// ---- downloadVerified retry ----

// TestDownloadVerifiedRetriesTransient5xx tests that a single 5xx blip
// during a metadata/package download is absorbed by the retry layer.
func TestDownloadVerifiedRetriesTransient5xx(t *testing.T) {
	payload := []byte("rpm-ish payload")
	sum := sha256.Sum256(payload)

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "pkg.rpm")

	err := downloadVerified(context.Background(), srv.URL, dest, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatalf("downloadVerified failed: %v", err)
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

// ---- downloadRPM mirror failover ----

// TestDownloadRPMMirrorFailover tests that a package download resolved via
// a mirrorlist placeholder fails over from a dead mirror to a healthy one.
func TestDownloadRPMMirrorFailover(t *testing.T) {
	payload := []byte("fake rpm body")
	sum := sha256.Sum256(payload)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer dead.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pkgs/test-1.0.rpm" {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write(payload)
	}))
	defer good.Close()

	mirrorlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s/\n%s/\n", dead.URL, good.URL)
	}))
	defer mirrorlist.Close()

	pkg := &PackageInfo{
		Name:         "test",
		BaseURL:      "mirrorlist:" + mirrorlist.URL,
		LocationHref: "pkgs/test-1.0.rpm",
		SHA256:       hex.EncodeToString(sum[:]),
	}

	dest, err := downloadRPM(context.Background(), pkg, t.TempDir())
	if err != nil {
		t.Fatalf("downloadRPM failed: %v", err)
	}

	data, err := os.ReadFile(dest) //nolint:gosec
	if err != nil {
		t.Fatalf("read downloaded rpm: %v", err)
	}

	if !bytes.Equal(data, payload) {
		t.Error("downloaded payload mismatch")
	}
}
