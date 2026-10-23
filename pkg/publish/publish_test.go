// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package publish_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/publish"
)

const testToken = "oa_dep_0123456789abcdef"

func TestMain(m *testing.M) {
	httpclient.SetRetryPolicy(3, time.Millisecond)
	os.Exit(m.Run())
}

func sum(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}

// storedPkg is a package held by the fake server.
type storedPkg struct {
	name, filename, sha string
}

// fakeOA is a minimal OpenArtifact: the deb/rpm upload APIs, the package
// listing and the raw repository routes, all behind Bearer authentication.
type fakeOA struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	pkgs     map[string][]storedPkg // repo -> packages
	raw      map[string][]byte      // "repo/path" -> bytes
	requests []string               // "METHOD path"
	fields   map[string]string      // last multipart text fields
	fileSeen []byte                 // last multipart file bytes
	failNext int                    // answer this many upload POSTs with 503 first
	lieSHA   string                 // report this sha256 instead of the real one
	listName []string               // package_name query values seen
}

func newFakeOA(t *testing.T) *fakeOA {
	t.Helper()

	f := &fakeOA{t: t, pkgs: map[string][]storedPkg{}, raw: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeOA) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeOA) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, r.Method+" "+r.URL.Path)

	if r.Header.Get("Authorization") != "Bearer "+testToken {
		f.writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "error": "unauthorized"})

		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case len(parts) == 5 && parts[0] == "api" && parts[4] == "debs" && r.Method == http.MethodPost:
		f.upload(w, r, parts[3])
	case len(parts) == 5 && parts[0] == "api" && parts[4] == "packages" && r.Method == http.MethodPost:
		f.upload(w, r, parts[3])
	case len(parts) == 5 && parts[0] == "api" && parts[4] == "packages" && r.Method == http.MethodGet:
		f.list(w, r, parts[3])
	case len(parts) >= 4 && parts[0] == "repos" && parts[1] == "raw":
		f.rawRoute(w, r, parts[2], strings.Join(parts[3:], "/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeOA) upload(w http.ResponseWriter, r *http.Request, repo string) {
	if f.failNext > 0 {
		f.failNext--
		f.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"success": false, "error": "busy"})

		return
	}

	if r.ContentLength <= 0 {
		f.t.Errorf("multipart upload must carry a Content-Length, got %d", r.ContentLength)
	}

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		f.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})

		return
	}

	f.fields = map[string]string{}
	for k, v := range r.MultipartForm.Value {
		f.fields[k] = v[0]
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		f.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "missing file field"})

		return
	}

	defer func() { _ = file.Close() }()

	data, _ := io.ReadAll(file)
	f.fileSeen = data
	s := sum(data)

	for _, p := range f.pkgs[repo] {
		if p.sha == s || p.filename == hdr.Filename {
			f.writeJSON(w, http.StatusConflict, map[string]any{
				"success": false, "error": "artifact already exists", "code": "CONFLICT",
				"data": map[string]any{"duplicate": true},
			})

			return
		}
	}

	f.pkgs[repo] = append(f.pkgs[repo], storedPkg{name: nameOf(hdr.Filename), filename: hdr.Filename, sha: s})

	reported := s
	if f.lieSHA != "" {
		reported = f.lieSHA
	}

	f.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    map[string]any{"sha256": reported, "size": len(data)},
	})
}

func nameOf(filename string) string {
	if n, _, ok := strings.Cut(filename, "_"); ok {
		return n
	}

	n, _, _ := strings.Cut(filename, "-")

	return n
}

func (f *fakeOA) list(w http.ResponseWriter, r *http.Request, repo string) {
	want := r.URL.Query().Get("package_name")
	f.listName = append(f.listName, want)

	var out []map[string]any

	for _, p := range f.pkgs[repo] {
		if want == "" || p.name == want {
			out = append(out, map[string]any{"name": p.name, "sha256": p.sha, "filename": p.filename})
		}
	}

	f.writeJSON(w, http.StatusOK, map[string]any{
		"total": len(out), "page": 1, "limit": 100, "packages": out,
	})
}

func (f *fakeOA) rawRoute(w http.ResponseWriter, r *http.Request, repo, rel string) {
	key := repo + "/" + rel

	switch r.Method {
	case http.MethodHead:
		data, ok := f.raw[key]
		if !ok {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("X-Checksum-Sha256", sum(data))
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		data, _ := io.ReadAll(r.Body)
		f.raw[key] = data
		f.writeJSON(w, http.StatusCreated, map[string]any{"path": rel, "sha256": sum(data), "size": len(data)})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeOA) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0

	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}

	return n
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func newPublisher(t *testing.T, f *fakeOA, mutate func(*publish.Config)) *publish.Publisher {
	t.Helper()

	cfg := publish.Config{
		URL: f.srv.URL, Token: testToken,
		DebRepo: "debs", RPMRepo: "rpms", APKRepo: "apks", PacmanRepo: "arch",
		Distribution: "jammy", Component: "universe",
	}
	if mutate != nil {
		mutate(&cfg)
	}

	p, err := publish.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func TestPublishDebUploadsWithDistributionAndComponent(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	dir := t.TempDir()
	deb := writeFile(t, dir, "hello_1.0_amd64.deb", "deb-bytes")

	res, err := newPublisher(t, f, nil).Publish(t.Context(), deb)
	if err != nil {
		t.Fatal(err)
	}

	if len(res) != 1 || res[0].Status != publish.StatusUploaded || res[0].SHA256 != sum([]byte("deb-bytes")) {
		t.Fatalf("unexpected result %+v", res)
	}

	if f.fields["distribution"] != "jammy" || f.fields["component"] != "universe" {
		t.Errorf("fields = %v", f.fields)
	}

	if string(f.fileSeen) != "deb-bytes" {
		t.Errorf("server received %q", f.fileSeen)
	}

	if f.count("POST /api/v1/repositories/debs/debs") != 1 {
		t.Errorf("requests = %v", f.requests)
	}
}

func TestPublishDefaultsDistributionAndComponent(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	deb := writeFile(t, t.TempDir(), "a_1_all.deb", "x")

	p := newPublisher(t, f, func(c *publish.Config) { c.Distribution, c.Component = "", "" })
	if _, err := p.Publish(t.Context(), deb); err != nil {
		t.Fatal(err)
	}

	if f.fields["distribution"] != "stable" || f.fields["component"] != "main" {
		t.Errorf("fields = %v", f.fields)
	}
}

func TestPublishRPMUsesPackagesRoute(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	rpm := writeFile(t, t.TempDir(), "hello-1.0-1.x86_64.rpm", "rpm-bytes")

	res, err := newPublisher(t, f, nil).Publish(t.Context(), rpm)
	if err != nil {
		t.Fatal(err)
	}

	if res[0].Status != publish.StatusUploaded || f.count("POST /api/v1/repositories/rpms/packages") != 1 {
		t.Fatalf("result %+v requests %v", res, f.requests)
	}

	if _, has := f.fields["distribution"]; has {
		t.Errorf("rpm upload must not send distribution: %v", f.fields)
	}
}

func TestPublishIdempotentOnConflictWithSameBytes(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	deb := writeFile(t, t.TempDir(), "hello_1.0_amd64.deb", "same")
	p := newPublisher(t, f, nil)

	if _, err := p.Publish(t.Context(), deb); err != nil {
		t.Fatal(err)
	}

	res, err := p.Publish(t.Context(), deb)
	if err != nil {
		t.Fatalf("second publish must succeed: %v", err)
	}

	if res[0].Status != publish.StatusUnchanged {
		t.Errorf("status = %s", res[0].Status)
	}

	if len(f.listName) == 0 || f.listName[0] != "hello" {
		t.Errorf("listing should be narrowed by package_name, saw %v", f.listName)
	}
}

func TestPublishConflictWithDifferentBytesFails(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	dir := t.TempDir()
	p := newPublisher(t, f, nil)

	first := writeFile(t, dir, "hello_1.0_amd64.deb", "v1")
	if _, err := p.Publish(t.Context(), first); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	changed := writeFile(t, other, "hello_1.0_amd64.deb", "v2-different")

	_, err := p.Publish(t.Context(), changed)
	if !errors.Is(err, publish.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestPublishConflictFallsBackToFullListingWhenNameGuessIsWrong(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	// Server stores the package under a different name than the file name suggests.
	f.pkgs["debs"] = []storedPkg{{name: "real-name", filename: "weird.deb", sha: sum([]byte("body"))}}
	deb := writeFile(t, t.TempDir(), "weird_1.0_amd64.deb", "body")

	// Force the 409 by pre-registering the filename the server compares.
	f.pkgs["debs"][0].filename = "weird_1.0_amd64.deb"

	res, err := newPublisher(t, f, nil).Publish(t.Context(), deb)
	if err != nil {
		t.Fatal(err)
	}

	if res[0].Status != publish.StatusUnchanged {
		t.Errorf("status = %s", res[0].Status)
	}

	if len(f.listName) != 2 || f.listName[0] != "weird" || f.listName[1] != "" {
		t.Errorf("expected hinted then full listing, saw %v", f.listName)
	}
}

func TestPublishRetriesTransientFailures(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	f.failNext = 2
	deb := writeFile(t, t.TempDir(), "r_1_all.deb", "retry")

	res, err := newPublisher(t, f, nil).Publish(t.Context(), deb)
	if err != nil {
		t.Fatal(err)
	}

	if res[0].Status != publish.StatusUploaded || f.count("POST ") != 3 {
		t.Errorf("status %s, requests %v", res[0].Status, f.requests)
	}
}

func TestPublishGivesUpAfterRetryBudget(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	f.failNext = 100
	deb := writeFile(t, t.TempDir(), "r_1_all.deb", "retry")

	_, err := newPublisher(t, f, nil).Publish(t.Context(), deb)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("want 503 error, got %v", err)
	}

	if f.count("POST ") != 3 {
		t.Errorf("expected 3 attempts, got %v", f.requests)
	}
}

func TestPublishChecksumMismatchFromServer(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	f.lieSHA = strings.Repeat("0", 64)
	deb := writeFile(t, t.TempDir(), "c_1_all.deb", "bytes")

	_, err := newPublisher(t, f, nil).Publish(t.Context(), deb)
	if !errors.Is(err, publish.ErrChecksumMismatch) {
		t.Fatalf("want ErrChecksumMismatch, got %v", err)
	}
}

func TestPublishAuthFailureIsNotRetriedAndNeverLeaksToken(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	deb := writeFile(t, t.TempDir(), "a_1_all.deb", "x")

	p := newPublisher(t, f, func(c *publish.Config) { c.Token = "wrong-secret-value" })

	_, err := p.Publish(t.Context(), deb)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("want 401 with server message, got %v", err)
	}

	if strings.Contains(err.Error(), "wrong-secret-value") {
		t.Errorf("token leaked into error: %v", err)
	}

	if f.count("POST ") != 1 {
		t.Errorf("401 must not be retried: %v", f.requests)
	}
}

func TestPublishNoRepositoryForFormat(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	rpm := writeFile(t, t.TempDir(), "a-1-1.noarch.rpm", "x")

	p := newPublisher(t, f, func(c *publish.Config) { c.RPMRepo = "" })

	_, err := p.Publish(t.Context(), rpm)
	if !errors.Is(err, publish.ErrNoRepository) {
		t.Fatalf("want ErrNoRepository, got %v", err)
	}
}

func TestPublishRawUploadsPackageAndSignature(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	dir := t.TempDir()
	pkg := writeFile(t, dir, "hello-1.0-1-x86_64.pkg.tar.zst", "pacman-bytes")
	writeFile(t, dir, "hello-1.0-1-x86_64.pkg.tar.zst.sig", "sig-bytes")

	p := newPublisher(t, f, func(c *publish.Config) { c.PacmanPath = "/os/x86_64/" })

	res, err := p.Publish(t.Context(), pkg)
	if err != nil {
		t.Fatal(err)
	}

	if len(res) != 2 || res[0].Status != publish.StatusUploaded || res[1].Status != publish.StatusUploaded {
		t.Fatalf("results %+v", res)
	}

	if got := string(f.raw["arch/os/x86_64/hello-1.0-1-x86_64.pkg.tar.zst"]); got != "pacman-bytes" {
		t.Errorf("package stored as %q", got)
	}

	if got := string(f.raw["arch/os/x86_64/hello-1.0-1-x86_64.pkg.tar.zst.sig"]); got != "sig-bytes" {
		t.Errorf("signature stored as %q", got)
	}
}

func TestPublishRawSkipsIdenticalBytes(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	apk := writeFile(t, t.TempDir(), "hello-1.0-r0.apk", "apk-bytes")
	p := newPublisher(t, f, nil)

	if _, err := p.Publish(t.Context(), apk); err != nil {
		t.Fatal(err)
	}

	res, err := p.Publish(t.Context(), apk)
	if err != nil {
		t.Fatal(err)
	}

	if res[0].Status != publish.StatusUnchanged || f.count("PUT ") != 1 {
		t.Errorf("status %s requests %v", res[0].Status, f.requests)
	}
}

func TestPublishRawReplacesChangedBytes(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	p := newPublisher(t, f, nil)
	a := writeFile(t, t.TempDir(), "x-1-r0.apk", "one")
	b := writeFile(t, t.TempDir(), "x-1-r0.apk", "two")

	if _, err := p.Publish(t.Context(), a); err != nil {
		t.Fatal(err)
	}

	res, err := p.Publish(t.Context(), b)
	if err != nil || res[0].Status != publish.StatusUploaded {
		t.Fatalf("res %+v err %v", res, err)
	}

	if string(f.raw["apks/x-1-r0.apk"]) != "two" {
		t.Errorf("stored %q", f.raw["apks/x-1-r0.apk"])
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := map[string]publish.Config{
		"missing url":      {Token: "t"},
		"missing token":    {URL: "https://oa.example.com"},
		"bad scheme":       {URL: "ftp://oa.example.com", Token: "t"},
		"userinfo":         {URL: "https://u:p@oa.example.com", Token: "t"},
		"plain http host":  {URL: "http://oa.example.com", Token: "t"},
		"not an url":       {URL: "://", Token: "t"},
		"no host in url":   {URL: "https://", Token: "t"},
		"relative url":     {URL: "oa.example.com", Token: "t"},
		"whitespace token": {URL: "https://oa.example.com"},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := publish.New(&cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewAcceptsHTTPSLoopbackAndExplicitInsecure(t *testing.T) {
	t.Parallel()

	ok := []publish.Config{
		{URL: "https://oa.example.com/", Token: "t"},
		{URL: "http://127.0.0.1:8080", Token: "t"},
		{URL: "http://localhost:8080", Token: "t"},
		{URL: "http://[::1]:8080", Token: "t"},
		{URL: "http://oa.internal", Token: "t", AllowInsecureHTTP: true},
	}

	for _, cfg := range ok {
		if _, err := publish.New(&cfg); err != nil {
			t.Errorf("%s: %v", cfg.URL, err)
		}
	}
}

func TestPublishHonoursBasePathPrefix(t *testing.T) {
	t.Parallel()

	f := newFakeOA(t)
	f.srv.Config.Handler = http.StripPrefix("/oa", http.HandlerFunc(f.handle))
	deb := writeFile(t, t.TempDir(), "a_1_all.deb", "x")

	p := newPublisher(t, f, func(c *publish.Config) { c.URL = f.srv.URL + "/oa/" })
	if _, err := p.Publish(t.Context(), deb); err != nil {
		t.Fatal(err)
	}
}

func TestSecretNeverRendersItsValue(t *testing.T) {
	t.Parallel()

	s := publish.Secret("super-secret")
	cfg := publish.Config{URL: "https://x", Token: s}

	var logs strings.Builder

	slog.New(slog.NewTextHandler(&logs, nil)).Info("cfg", "token", s, "cfg", cfg)

	js, _ := json.Marshal(map[string]any{"t": s, "c": cfg})

	for name, out := range map[string]string{
		"%v|%s": fmt.Sprintf("%v|%s", s, s), "%+v": fmt.Sprintf("%+v", cfg), "%#v": fmt.Sprintf("%#v", s),
		"%q": fmt.Sprintf("%q", s), "slog": logs.String(), "json": string(js),
	} {
		if strings.Contains(out, "super-secret") {
			t.Errorf("%s leaked the secret: %s", name, out)
		}
	}

	if s.Reveal() != "super-secret" {
		t.Error("Reveal must return the raw value")
	}
}

func TestApplyEnv(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		publish.EnvURL: " https://oa.example.com ", publish.EnvToken: "tok",
		publish.EnvDebRepo: "d", publish.EnvRPMRepo: "r", publish.EnvAPKRepo: "a", publish.EnvPacmanRepo: "p",
		publish.EnvDistribution: "noble", publish.EnvComponent: "contrib",
		publish.EnvAPKPath: "v3", publish.EnvPacmanPath: "x86_64",
	}
	cfg := publish.Config{URL: "https://old", DebRepo: "keep-if-env-empty", Component: "main"}

	if err := cfg.ApplyEnv(func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}

	want := publish.Config{
		URL: "https://oa.example.com", DebRepo: "d", RPMRepo: "r", APKRepo: "a", PacmanRepo: "p",
		Distribution: "noble", Component: "contrib", APKPath: "v3", PacmanPath: "x86_64", Token: "tok",
	}
	if cfg != want {
		t.Errorf("got %+v want %+v", cfg, want)
	}

	// Unset variables leave existing values alone.
	cfg2 := publish.Config{DebRepo: "stay"}
	if err := cfg2.ApplyEnv(func(string) string { return "" }); err != nil || cfg2.DebRepo != "stay" {
		t.Errorf("empty env changed config: %+v %v", cfg2, err)
	}
}

func TestTokenFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := writeFile(t, dir, "token", "  oa_dep_filetoken\n")

	var cfg publish.Config

	if err := cfg.ApplyEnv(func(k string) string {
		if k == publish.EnvTokenFile {
			return file
		}

		return ""
	}); err != nil {
		t.Fatal(err)
	}

	if cfg.Token.Reveal() != "oa_dep_filetoken" {
		t.Errorf("token = %q", cfg.Token.Reveal())
	}

	// YAP_OA_TOKEN wins over the file.
	if err := cfg.ApplyEnv(func(k string) string {
		return map[string]string{publish.EnvToken: "direct", publish.EnvTokenFile: file}[k]
	}); err != nil || cfg.Token.Reveal() != "direct" {
		t.Errorf("token = %q err %v", cfg.Token.Reveal(), err)
	}

	empty := writeFile(t, dir, "empty", "\n")
	if err := cfg.LoadTokenFile(empty); err == nil {
		t.Error("empty token file must fail")
	}

	if err := cfg.LoadTokenFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing token file must fail")
	}
}

func TestDetectFormat(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a_1.0_amd64.deb":                 publish.FormatDeb,
		"/x/A-1.0-1.x86_64.RPM":           publish.FormatRPM,
		"a-1.0-r0.apk":                    publish.FormatAPK,
		"a-1.0-1-x86_64.pkg.tar.zst":      publish.FormatPacman,
		"a-1.0-1-x86_64.pkg.tar.xz":       publish.FormatPacman,
		"a-1.0-1-x86_64.pkg.tar":          publish.FormatPacman,
		"a-1.0-1-x86_64.pkg.tar.zst.sig":  "",
		"a_1.0_amd64.deb.asc":             "",
		"a-1.0-1.x86_64.rpm.sbom.spdx":    "",
		"README.md":                       "",
		"a-1.0-1-x86_64.pkg.tar.zst.json": "",
	}

	for name, want := range tests {
		got, ok := publish.DetectFormat(name)
		if got != want || ok != (want != "") {
			t.Errorf("DetectFormat(%q) = %q,%v want %q", name, got, ok, want)
		}
	}
}

func TestCollectArtifacts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "b_1_all.deb", "x")
	writeFile(t, dir, "a_1_all.deb", "x")
	writeFile(t, dir, "a_1_all.deb.asc", "sig")
	writeFile(t, dir, "notes.txt", "x")

	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}

	files, fromDir, err := publish.CollectArtifacts([]string{dir, filepath.Join(dir, "b_1_all.deb")})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 2 || filepath.Base(files[0]) != "a_1_all.deb" || filepath.Base(files[1]) != "b_1_all.deb" {
		t.Fatalf("files = %v", files)
	}

	if !fromDir[files[0]] || !fromDir[files[1]] {
		t.Errorf("directory members must be flagged: %v", fromDir)
	}

	if _, _, err := publish.CollectArtifacts([]string{filepath.Join(dir, "notes.txt")}); err == nil {
		t.Error("explicit non-package file must fail")
	}

	if _, _, err := publish.CollectArtifacts([]string{filepath.Join(dir, "missing.deb")}); err == nil {
		t.Error("missing path must fail")
	}
}

func TestGuessPackageName(t *testing.T) {
	t.Parallel()

	tests := []struct{ format, file, want string }{
		{publish.FormatDeb, "/p/hello_1.0-1_amd64.deb", "hello"},
		{publish.FormatDeb, "nounderscore.deb", ""},
		{publish.FormatRPM, "hello-1.0-1.x86_64.rpm", "hello"},
		{publish.FormatRPM, "my-tool-2.3.4-5.fc40.noarch.rpm", "my-tool"},
		{publish.FormatRPM, "bad.rpm", ""},
		{publish.FormatAPK, "x.apk", ""},
	}

	for _, tt := range tests {
		if got := publish.GuessPackageName(tt.format, tt.file); got != tt.want {
			t.Errorf("GuessPackageName(%s,%s) = %q want %q", tt.format, tt.file, got, tt.want)
		}
	}
}
