// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package repo registers extra distribution repositories (apt sources or
// dnf/yum repos) on the build host before any package manager operation. It
// supports configuration via yap.json (top-level "repos" array) and via the
// repeatable `--repo key=val,...` command-line flag.
package repo

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// keyFetchTimeout bounds how long a GPG key download may take.
const keyFetchTimeout = 30 * time.Second

// maxKeyBytes caps the size of a downloaded GPG key (1 MiB).
const maxKeyBytes int64 = 1 << 20

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
	// Country is a two-letter country code (ISO 3166-1 alpha-2 style,
	// e.g. "it", "de"; only the shape is validated) substituted
	// for the "{country}" token in URL, selecting a country mirror:
	// url="http://{country}.archive.ubuntu.com/ubuntu/" + country="it"
	// → "http://it.archive.ubuntu.com/ubuntu/". When Country is empty the
	// token collapses ("{country}." is removed), falling back to the bare
	// host. Setting Country on a URL without the token is an error.
	Country string `json:"country,omitempty" validate:"omitempty,len=2,alpha"`
}

// Setup writes apt/dnf repository definitions for every Repo applicable to the
// current distro+release pair. distro is the bare os-release ID (e.g. "ubuntu",
// "rocky"); release is the codename or version (e.g. "jammy", "9"). Repos whose
// Distros list is empty match every distro. When Distros is non-empty, an entry
// matches if it equals the bare distro ("ubuntu") OR the qualified form
// ("ubuntu-jammy").
func Setup(distro, release string, repos []Repo) error {
	return SetupContext(context.Background(), distro, release, repos)
}

// SetupContext is Setup with a caller-supplied context that bounds key
// fetches and other network operations.
func SetupContext(ctx context.Context, distro, release string, repos []Repo) error {
	if len(repos) == 0 {
		return nil
	}

	pm := constants.DistroToPackageManager[distro]
	if pm == "" {
		logger.Warn(i18n.T("logger.repo.warn.unknown_distro_skipping_repo"), "distro", distro)

		return nil
	}

	for i := range repos {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := setupOneContext(ctx, pm, &repos[i], distro, release, i); err != nil {
			return err
		}
	}

	return nil
}

// setupOne installs a single repository definition if it targets the active
// distro and matches the active package format.
func setupOneContext(
	ctx context.Context, pm string, r *Repo, distro, release string, idx int,
) error {
	if !appliesTo(r, distro, release) {
		return nil
	}

	if r.Name == "" || r.URL == "" {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("repo: name and url are required (entry %d)", idx)).
			WithOperation("setupOne")
	}

	resolved := *r
	if err := resolveCountry(&resolved); err != nil {
		return err
	}

	if err := validateRepo(&resolved); err != nil {
		return err
	}

	r = &resolved

	format := r.Format
	if format == "" {
		format = formatFor(pm)
	}

	switch format {
	case formatDeb:
		if pm != constants.PMApt {
			return nil
		}

		return setupDebContext(ctx, r)
	case formatRPM:
		if pm != constants.PMYum && pm != constants.PMZypper {
			return nil
		}

		return setupRPMContext(ctx, r)
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

// resolveCountry substitutes the "{country}" token in r.URL with r.Country
// (ISO 3166-1 alpha-2, lowercased). With an empty Country only the
// host-prefix form "{country}." collapses safely to the global mirror;
// any other token placement cannot be removed without changing path
// semantics and is rejected. A Country set on a URL without the token is
// also rejected: it would silently do nothing.
func resolveCountry(r *Repo) error {
	const token = "{country}"

	hasToken := strings.Contains(r.URL, token)

	if r.Country == "" {
		if !hasToken {
			return nil
		}

		collapsed := strings.Replace(r.URL, "://"+token+".", "://", 1)
		if strings.Contains(collapsed, token) {
			return errors.New(errors.ErrTypeValidation,
				fmt.Sprintf("repo %q: url contains {country} but no country is set", r.Name)).
				WithOperation("resolveCountry")
		}

		r.URL = collapsed

		return nil
	}

	cc := strings.ToLower(r.Country)
	if len(cc) != 2 || cc[0] < 'a' || cc[0] > 'z' || cc[1] < 'a' || cc[1] > 'z' {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("repo %q: country must be a two-letter code, got %q", r.Name, r.Country)).
			WithOperation("resolveCountry")
	}

	if !hasToken {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("repo %q: country=%q set but url has no {country} token", r.Name, cc)).
			WithOperation("resolveCountry")
	}

	r.URL = strings.ReplaceAll(r.URL, token, cc)

	return nil
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
// escaped with '+'), keyURL, distros (use '+'), format, gpgCheck, country.
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
		case "country":
			r.Country = val
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
	return fetchKeyContext(context.Background(), url, dst)
}

// fetchKeyContext is fetchKey bounded by ctx. The downloaded content must parse
// as an OpenPGP keyring (armored or binary) before it replaces dst.
func fetchKeyContext(ctx context.Context, url, dst string) error {
	// /etc/apt/keyrings and /etc/pki/rpm-gpg are system-wide directories that
	// must remain traversable by the unprivileged _apt / dnf-update accounts.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, keyFetchTimeout)
	defer cancel()

	// FetchToFile enforces a size cap, retries transient failures and writes
	// via temp file + rename, so a failed download never leaves a truncated key.
	// Download to a sibling temp path so invalid content never replaces dst.
	tmp := dst + ".tmp"
	defer os.Remove(tmp) //nolint:errcheck

	if err := httpclient.FetchToFile(ctx, url, tmp, maxKeyBytes); err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork,
			"failed to fetch repo key").
			WithOperation("fetchKey").
			WithContext("url", url).
			WithContext("path", dst)
	}

	if err := validateKeyFile(tmp); err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation,
			"fetched repo key is not a valid OpenPGP keyring").
			WithOperation("fetchKey").
			WithContext("url", url)
	}

	if err := os.Rename(tmp, dst); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to install repo key").
			WithOperation("fetchKey").
			WithContext("path", dst)
	}

	// apt and dnf require world-readable keys so the unprivileged update
	// process can verify package signatures.
	if err := os.Chmod(dst, 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to set repo key permissions").
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

// validateKeyFile checks that path holds at least one OpenPGP key, armored or
// binary.
func validateKeyFile(path string) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}
	defer closeQuiet(f, path)

	head := make([]byte, 64)
	n, _ := io.ReadFull(f, head)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var list openpgp.EntityList
	if bytes.Contains(head[:n], []byte("-----BEGIN")) {
		list, err = openpgp.ReadArmoredKeyRing(f)
	} else {
		list, err = openpgp.ReadKeyRing(f)
	}

	if err != nil {
		return err
	}

	if len(list) == 0 {
		return stderrors.New("no OpenPGP keys found")
	}

	return nil
}
