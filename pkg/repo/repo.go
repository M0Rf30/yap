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
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// keyFetchTimeout bounds how long a GPG key download may take.
const keyFetchTimeout = 30 * time.Second

// Internal format identifiers used to dispatch Setup to the per-format writer
// and to gate yap.json `format` values supplied by users.
const (
	formatDeb     = "deb"
	formatRPM     = "rpm"
	componentMain = "main"
)

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
// current distro+release pair. distro is the bare os-release ID (e.g. "ubuntu",
// "rocky"); release is the codename or version (e.g. "jammy", "9"). Repos whose
// Distros list is empty match every distro. When Distros is non-empty, an entry
// matches if it equals the bare distro ("ubuntu") OR the qualified form
// ("ubuntu-jammy").
func Setup(distro, release string, repos []Repo) error {
	if len(repos) == 0 {
		return nil
	}

	pm := constants.DistroToPackageManager[distro]
	if pm == "" {
		logger.Warn(i18n.T("logger.repo.warn.unknown_distro_skipping_repo"), "distro", distro)

		return nil
	}

	for i := range repos {
		if err := setupOne(pm, &repos[i], distro, release, i); err != nil {
			return err
		}
	}

	return nil
}

// setupOne installs a single repository definition if it targets the active
// distro and matches the active package format.
func setupOne(pm string, r *Repo, distro, release string, idx int) error {
	if !appliesTo(r, distro, release) {
		return nil
	}

	if r.Name == "" || r.URL == "" {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("repo: name and url are required (entry %d)", idx)).
			WithOperation("setupOne")
	}

	format := r.Format
	if format == "" {
		format = formatFor(pm)
	}

	switch format {
	case formatDeb:
		if pm != constants.PMApt {
			return nil
		}

		return setupDeb(r)
	case formatRPM:
		if pm != constants.PMYum && pm != constants.PMZypper {
			return nil
		}

		return setupRPM(r)
	default:
		logger.Warn(i18n.T("logger.repo.warn.unsupported_format_skipping"), "name", r.Name,
			"format", format)

		return nil
	}
}

// appliesTo reports whether the repo declaration targets the given distro.
// When release is non-empty, entries may use either the bare distro key
// ("ubuntu") or the qualified "distro-release" form ("ubuntu-jammy").
func appliesTo(r *Repo, distro, release string) bool {
	if len(r.Distros) == 0 {
		return true
	}

	if slices.Contains(r.Distros, distro) {
		return true
	}

	if release != "" {
		return slices.Contains(r.Distros, distro+"-"+release)
	}

	return false
}

// formatFor returns the canonical repo format ("deb" or "rpm") for the active
// package manager. Unsupported package managers fall back to "deb".
func formatFor(pm string) string {
	switch pm {
	case constants.PMApt:
		return formatDeb
	case constants.PMYum, constants.PMZypper:
		return formatRPM
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
			return r, errors.New(errors.ErrTypeValidation,
				fmt.Sprintf("repo: expected key=val in %q", part)).
				WithOperation("parseFlag")
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
			return r, errors.New(errors.ErrTypeValidation,
				fmt.Sprintf("repo: unknown key %q", key)).
				WithOperation("parseFlag")
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
		return errors.Wrap(err, errors.ErrTypeNetwork,
			"failed to fetch repo key").
			WithOperation("fetchKey").
			WithContext("url", url)
	}
	defer closeQuiet(resp.Body, "key response body")

	if resp.StatusCode != http.StatusOK {
		return errors.New(errors.ErrTypeNetwork,
			"failed to fetch repo key").
			WithOperation("fetchKey").
			WithContext("url", url).
			WithContext("status", resp.StatusCode)
	}

	// dst is built from a constant directory plus a sanitized repo name; apt
	// and dnf require world-readable keys so the unprivileged update process
	// can verify package signatures.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec
	if err != nil {
		return err
	}
	defer closeQuiet(f, dst)

	if _, err := io.Copy(f, resp.Body); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write repo key").
			WithOperation("fetchKey").
			WithContext("path", dst)
	}

	return nil
}

// closeQuiet closes c and logs a warning instead of swallowing the error.
func closeQuiet(c io.Closer, what string) {
	if err := c.Close(); err != nil {
		logger.Warn(i18n.T("logger.repo.warn.close_failed"), "target", what, "error", err)
	}
}
