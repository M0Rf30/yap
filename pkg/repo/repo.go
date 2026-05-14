// Package repo registers extra distribution repositories (apt sources or
// dnf/yum repos) on the build host before any package manager operation. It
// supports configuration via yap.json (top-level "repos" array) and via the
// repeatable `--repo key=val,...` command-line flag.
package repo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// keyFetchTimeout bounds how long a GPG key download may take.
const keyFetchTimeout = 30 * time.Second

// Repo describes one additional package repository. The same structure is used
// for both yap.json declarations and CLI parsing. Fields that do not apply to
// a given format are ignored at write time.
type Repo struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Suite      string   `json:"suite,omitempty"`
	Components []string `json:"components,omitempty"`
	KeyURL     string   `json:"keyURL,omitempty"`
	Distros    []string `json:"distros,omitempty"`
	Format     string   `json:"format,omitempty"`
	GPGCheck   bool     `json:"gpgCheck,omitempty"`
}

// Setup writes apt/dnf repository definitions for every Repo applicable to the
// current distro. distro is the bare distro key (e.g. "ubuntu", "rocky"); only
// repos with empty Distros, or whose Distros entry matches, are installed.
// Repos that target a different package format than the active distro are
// silently skipped.
func Setup(distro string, repos []Repo) error {
	if len(repos) == 0 {
		return nil
	}

	pm := constants.DistroToPackageManager[distro]
	if pm == "" {
		logger.Warn("repo: unknown distro, skipping repo setup", "distro", distro)

		return nil
	}

	for i := range repos {
		if err := setupOne(pm, &repos[i], distro, i); err != nil {
			return err
		}
	}

	return nil
}

// setupOne installs a single repository definition if it targets the active
// distro and matches the active package format.
func setupOne(pm string, r *Repo, distro string, idx int) error {
	if !appliesTo(r, distro) {
		return nil
	}

	if r.Name == "" || r.URL == "" {
		return fmt.Errorf("repo: name and url are required (entry %d)", idx)
	}

	format := r.Format
	if format == "" {
		format = formatFor(pm)
	}

	switch format {
	case "deb":
		if pm != constants.PMApt {
			return nil
		}

		return setupDeb(r)
	case "rpm":
		if pm != constants.PMYum && pm != constants.PMZypper {
			return nil
		}

		return setupRPM(r)
	default:
		logger.Warn("repo: unsupported format, skipping",
			"name", r.Name,
			"format", format)

		return nil
	}
}

// appliesTo reports whether the repo declaration targets the given distro.
func appliesTo(r *Repo, distro string) bool {
	if len(r.Distros) == 0 {
		return true
	}

	return slices.Contains(r.Distros, distro)
}

// formatFor returns the canonical repo format ("deb" or "rpm") for the active
// package manager. Unsupported package managers fall back to "deb".
func formatFor(pm string) string {
	switch pm {
	case constants.PMApt:
		return "deb"
	case constants.PMYum, constants.PMZypper:
		return "rpm"
	}

	return ""
}

// ParseFlags converts repeatable `--repo k=v,...` tokens into Repo values.
// Supported keys: name, url, suite, components (comma-separated subkeys are
// escaped with '+'), keyURL, distros (use '+'), format, gpgCheck.
func ParseFlags(tokens []string) ([]Repo, error) {
	out := make([]Repo, 0, len(tokens))

	for _, t := range tokens {
		r, err := parseFlag(t)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, nil
}

func parseFlag(token string) (Repo, error) {
	var r Repo

	for part := range strings.SplitSeq(token, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return r, fmt.Errorf("repo: expected key=val in %q", part)
		}

		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "name":
			r.Name = val
		case "url":
			r.URL = val
		case "suite":
			r.Suite = val
		case "components":
			r.Components = splitPlus(val)
		case "keyURL", "key":
			r.KeyURL = val
		case "distros":
			r.Distros = splitPlus(val)
		case "format":
			r.Format = val
		case "gpgCheck":
			r.GPGCheck = val == "true" || val == "1"
		default:
			return r, fmt.Errorf("repo: unknown key %q", key)
		}
	}

	return r, nil
}

func splitPlus(s string) []string {
	parts := strings.Split(s, "+")

	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// fetchKey downloads the GPG key referenced by KeyURL to dst. Existing files
// are overwritten so re-runs pick up rotated keys.
func fetchKey(url, dst string) error {
	// /etc/apt/keyrings and /etc/pki/rpm-gpg are system-wide directories that
	// must remain traversable by the unprivileged _apt / dnf-update accounts.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { // #nosec G301
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), keyFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("repo: fetch key %s: %w", url, err)
	}
	defer closeQuiet(resp.Body, "key response body")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("repo: fetch key %s: status %d", url, resp.StatusCode)
	}

	// dst is built from a constant directory plus a sanitized repo name; apt
	// and dnf require world-readable keys so the unprivileged update process
	// can verify package signatures.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) // #nosec G302,G304
	if err != nil {
		return err
	}
	defer closeQuiet(f, dst)

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("repo: write key %s: %w", dst, err)
	}

	return nil
}

// closeQuiet closes c and logs a warning instead of swallowing the error.
func closeQuiet(c io.Closer, what string) {
	if err := c.Close(); err != nil {
		logger.Warn("repo: close failed", "target", what, "error", err)
	}
}
