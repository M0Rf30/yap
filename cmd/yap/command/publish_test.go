// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// newPublishTestCmd returns a fresh command bound to the shared pubFlags and
// resets every flag to its default, so tests do not leak state.
func newPublishTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	pubFlags = publishFlags{retries: publishDefaultTry}

	c := &cobra.Command{Use: "publish", RunE: runPublish, Args: cobra.ArbitraryArgs}
	f := c.Flags()
	f.StringVar(&pubFlags.config, "config", "", "")
	f.StringVar(&pubFlags.url, "url", "", "")
	f.StringVar(&pubFlags.tokenFile, "token-file", "", "")
	f.StringVar(&pubFlags.debRepo, "deb-repo", "", "")
	f.StringVar(&pubFlags.rpmRepo, "rpm-repo", "", "")
	f.StringVar(&pubFlags.apkRepo, "apk-repo", "", "")
	f.StringVar(&pubFlags.pacmanRepo, "pacman-repo", "", "")
	f.StringVar(&pubFlags.distribution, "distribution", "", "")
	f.StringVar(&pubFlags.component, "component", "", "")
	f.StringVar(&pubFlags.apkPath, "apk-path", "", "")
	f.StringVar(&pubFlags.pacmanPath, "pacman-path", "", "")
	f.BoolVar(&pubFlags.insecureHTTP, "insecure-http", false, "")
	f.BoolVar(&pubFlags.dryRun, "dry-run", false, "")
	f.IntVar(&pubFlags.retries, "retries", publishDefaultTry, "")
	f.DurationVar(&pubFlags.timeout, "timeout", time.Minute, "")

	return c
}

func clearPublishEnv(t *testing.T) {
	t.Helper()

	for _, k := range []string{
		"YAP_OA_URL", "YAP_OA_TOKEN", "YAP_OA_TOKEN_FILE", "YAP_OA_DEB_REPO", "YAP_OA_RPM_REPO",
		"YAP_OA_APK_REPO", "YAP_OA_PACMAN_REPO", "YAP_OA_DISTRIBUTION", "YAP_OA_COMPONENT",
		"YAP_OA_APK_PATH", "YAP_OA_PACMAN_PATH",
	} {
		t.Setenv(k, "")
	}
}

func TestResolvePublishConfigPrecedence(t *testing.T) {
	clearPublishEnv(t)

	dir := t.TempDir()
	projectFile := filepath.Join(dir, "yap.json")
	require.NoError(t, os.WriteFile(projectFile, []byte(`{
		"output": "dist",
		"publish": {"url": "https://file.example", "debRepo": "file-debs", "rpmRepo": "file-rpms",
		            "distribution": "file-dist", "token": "must-be-ignored"}
	}`), 0o600))

	t.Setenv("YAP_OA_URL", "https://env.example")
	t.Setenv("YAP_OA_RPM_REPO", "env-rpms")
	t.Setenv("YAP_OA_TOKEN", "env-token")

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags([]string{"--config", projectFile, "--rpm-repo", "flag-rpms"}))

	cfg, output, err := resolvePublishConfig(c)
	require.NoError(t, err)

	assert.Equal(t, "https://env.example", cfg.URL, "env overrides yap.json")
	assert.Equal(t, "file-debs", cfg.DebRepo, "yap.json value kept when nothing overrides it")
	assert.Equal(t, "flag-rpms", cfg.RPMRepo, "flag overrides env")
	assert.Equal(t, "file-dist", cfg.Distribution)
	assert.Equal(t, "env-token", cfg.Token.Reveal())
	assert.Equal(t, filepath.Join(dir, "dist"), output, "output resolved against yap.json dir")
}

func TestResolvePublishConfigTokenFileFlagWinsOverEnvToken(t *testing.T) {
	clearPublishEnv(t)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-token\n"), 0o600))
	t.Setenv("YAP_OA_TOKEN", "env-token")

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags([]string{"--token-file", tokenFile}))

	cfg, _, err := resolvePublishConfig(c)
	require.NoError(t, err)
	assert.Equal(t, "file-token", cfg.Token.Reveal())
}

func TestRunPublishDryRunNeedsNoServerOrToken(t *testing.T) {
	clearPublishEnv(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_1_all.deb"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a-1-1.noarch.rpm"), []byte("x"), 0o600))

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags([]string{"--dry-run", "--deb-repo", "debs"}))
	assert.NoError(t, runPublish(c, []string{dir}), "rpm has no repo but comes from a directory: skipped")

	// An explicitly named file with no repository is an error.
	err := runPublish(c, []string{filepath.Join(dir, "a-1-1.noarch.rpm")})
	assert.ErrorContains(t, err, "no repository configured")
}

func TestRunPublishEndToEnd(t *testing.T) {
	clearPublishEnv(t)
	httpclient.SetRetryPolicy(2, time.Millisecond)

	var posts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v1/repositories/debs/debs", r.URL.Path)
		posts.Add(1)

		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, _, err := r.FormFile("file")
		require.NoError(t, err)

		defer func() { _ = f.Close() }()

		w.Header().Set("Content-Type", "application/json")
		// sha256("x")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"sha256": "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_1_all.deb"), []byte("x"), 0o600))

	t.Setenv("YAP_OA_TOKEN", "tok")

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags([]string{"--url", srv.URL, "--deb-repo", "debs"}))
	require.NoError(t, runPublish(c, []string{dir}))
	assert.EqualValues(t, 1, posts.Load())
}

func TestRunPublishReportsFailure(t *testing.T) {
	clearPublishEnv(t)
	httpclient.SetRetryPolicy(1, time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "a_1_all.deb")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	t.Setenv("YAP_OA_TOKEN", "tok")

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags([]string{"--url", srv.URL, "--deb-repo", "debs", "--retries", "1"}))
	assert.Error(t, runPublish(c, []string{file}))
}

func TestRunPublishRequiresArtifactsOrOutput(t *testing.T) {
	clearPublishEnv(t)

	c := newPublishTestCmd(t)
	require.NoError(t, c.ParseFlags(nil))

	t.Chdir(t.TempDir())
	assert.Error(t, runPublish(c, nil))
}
