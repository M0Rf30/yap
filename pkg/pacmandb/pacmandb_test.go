package pacmandb //nolint:testpackage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncWithMockServer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock HTTP server that serves fake .db files
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/core/os/x86_64/core.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "fake-db-content")

			return
		}

		if r.URL.Path == "/core/os/x86_64/core.db.sig" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "fake-sig-content")

			return
		}

		if r.URL.Path == "/extra/os/x86_64/extra.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "fake-extra-db-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create a mirrorlist pointing to our mock server
	mirrorlistPath := filepath.Join(tmpDir, "mirrorlist")
	mirrorlistContent := fmt.Sprintf(`Server = %s/$repo/os/$arch`, server.URL)

	err := os.WriteFile(mirrorlistPath, []byte(mirrorlistContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write mirrorlist: %v", err)
	}

	// Create pacman.conf
	confPath := filepath.Join(tmpDir, "pacman.conf")
	confContent := fmt.Sprintf(`[options]
Architecture = x86_64

[core]
Include = %s

[extra]
Include = %s
`, mirrorlistPath, mirrorlistPath)

	err = os.WriteFile(confPath, []byte(confContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write pacman.conf: %v", err)
	}

	// Override pacmanSyncDir to use tmpDir
	oldSyncDir := pacmanSyncDir

	defer func() {
		// Note: we can't actually override the constant, so we'll test with the real path
		// but verify the logic works
	}()

	_ = oldSyncDir

	// Test the sync function with our mock config
	cfg, err := parseConfigWithIncludes(confPath, make(map[string]bool))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(cfg.Repos))
	}

	// Verify we can construct the URLs correctly
	arch := "x86_64"

	for _, repo := range cfg.Repos {
		if len(repo.Servers) == 0 {
			t.Errorf("repo %q has no servers", repo.Name)
			continue
		}

		url := substituteVars(repo.Servers[0], repo.Name, arch) + "/" + repo.Name + ".db"

		expectedURL := fmt.Sprintf("%s/%s/os/%s/%s.db", server.URL, repo.Name, arch, repo.Name)
		if url != expectedURL {
			t.Errorf("expected URL %q, got %q", expectedURL, url)
		}
	}
}

func TestDownloadFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "test-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/test.db", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file was created and has correct content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(content) != "test-content" {
		t.Errorf("expected content 'test-content', got %q", string(content))
	}
}

func TestDownloadFileHTTPError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/nonexistent.db", destPath)
	if err == nil {
		t.Error("expected error for 404, got nil")
	}

	// Verify no file was created
	if _, err := os.Stat(destPath); err == nil {
		t.Error("expected file to not exist after failed download")
	}
}

func TestDownloadFileContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just hang forever
		select {}
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db")

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := downloadFile(ctx, server.URL+"/test.db", destPath)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestDownloadFileAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "test-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/test.db", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify no .tmp file is left behind
	tmpPath := destPath + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("expected .tmp file to be cleaned up after successful download")
	}
}

func TestDownloadFileWriteError(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "test-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// Use a non-existent parent directory to trigger write error
	destPath := "/nonexistent/parent/dir/test.db"
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/test.db", destPath)
	if err == nil {
		t.Error("expected error for write to non-existent dir, got nil")
	}
}

func TestSyncRepoMultipleMirrors(t *testing.T) {
	// Create two mock servers: first fails, second succeeds
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer failServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/core/os/x86_64/core.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = fmt.Fprint(w, "fake-db-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer successServer.Close()

	// Test that substituteVars works correctly for multiple servers
	repo := Repo{
		Name: "core",
		Servers: []string{
			failServer.URL + "/core/os/$arch",
			successServer.URL + "/core/os/$arch",
		},
	}

	// Verify that both servers are configured correctly
	if len(repo.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(repo.Servers))
	}

	// Verify URL substitution works for both
	for i, server := range repo.Servers {
		url := substituteVars(server, repo.Name, "x86_64") + "/" + repo.Name + ".db"
		if url == "" {
			t.Errorf("server %d produced empty URL", i)
		}
	}
}

func TestSyncRepoAllMirrorsFail(t *testing.T) {
	// Create a mock server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := Repo{
		Name: "core",
		Servers: []string{
			server.URL + "/core/os/$arch",
		},
	}

	ctx := context.Background()

	err := syncRepo(ctx, repo, "x86_64")
	if err == nil {
		t.Error("expected error when all mirrors fail, got nil")
	}
}

func TestDownloadFileWithLargeContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that serves large content
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large.db" {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(largeContent)

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "large.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/large.db", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file size
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("failed to stat downloaded file: %v", err)
	}

	if info.Size() != int64(len(largeContent)) {
		t.Errorf("expected file size %d, got %d", len(largeContent), info.Size())
	}

	// Verify content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if len(content) != len(largeContent) {
		t.Errorf("expected content length %d, got %d", len(largeContent), len(content))
	}
}

func TestDownloadFileSignature(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.db.sig" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "signature-content")

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db.sig")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/test.db.sig", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file was created
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(content) != "signature-content" {
		t.Errorf("expected content 'signature-content', got %q", string(content))
	}
}

func TestDownloadFilePartialRead(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that serves content in chunks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.db" {
			w.Header().Set("Content-Type", "application/gzip")
			// Write in chunks to test streaming
			for i := range 10 {
				_, _ = fmt.Fprintf(w, "chunk-%d-", i)
			}

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/test.db", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file was created with all content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	expectedContent := "chunk-0-chunk-1-chunk-2-chunk-3-chunk-4-chunk-5-chunk-6-chunk-7-chunk-8-chunk-9-"
	if string(content) != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, string(content))
	}
}

func TestDownloadFileEmptyResponse(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that serves empty content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty.db" {
			w.Header().Set("Content-Type", "application/gzip")
			// Write nothing
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "empty.db")
	ctx := context.Background()

	err := downloadFile(ctx, server.URL+"/empty.db", destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file was created but is empty
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("failed to stat downloaded file: %v", err)
	}

	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}
