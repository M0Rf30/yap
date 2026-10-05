// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package repo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// repoNameRe restricts repo names to a safe file-name/section-name subset.
// The first character must be alphanumeric so names can never be "." or "..".
var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// debTokenRe matches a single deb822 suite or component token.
var debTokenRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+-]*$`)

// validateRepo rejects repo fields that would escape the target directory or
// inject extra directives into the generated .repo / .sources files.
func validateRepo(r *Repo) error {
	if !repoNameRe.MatchString(r.Name) {
		return invalidRepoField(r, "name", "must match "+repoNameRe.String())
	}

	if reason := validateHTTPURL(r.URL); reason != "" {
		return invalidRepoField(r, "url", reason)
	}

	if r.KeyURL != "" {
		if reason := validateHTTPURL(r.KeyURL); reason != "" {
			return invalidRepoField(r, "keyURL", reason)
		}
	}

	if hasControl(r.Suite) {
		return invalidRepoField(r, "suite", "contains control characters")
	}

	for tok := range strings.FieldsSeq(r.Suite) {
		if !debTokenRe.MatchString(tok) {
			return invalidRepoField(r, "suite", fmt.Sprintf("invalid token %q", tok))
		}
	}

	for _, c := range r.Components {
		if !debTokenRe.MatchString(c) {
			return invalidRepoField(r, "components", fmt.Sprintf("invalid component %q", c))
		}
	}

	return nil
}

// validateHTTPURL requires an http(s) URL without whitespace/control chars.
// It returns a human-readable reason, or "" when raw is acceptable.
func validateHTTPURL(raw string) string {
	if hasControl(raw) || strings.ContainsAny(raw, " \t") {
		return "contains whitespace or control characters"
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "not a valid URL"
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "must be an http(s) URL with a host"
	}

	return ""
}

// hasControl reports whether s contains ASCII control characters (including
// CR/LF, which would allow directive injection).
func hasControl(s string) bool {
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}

	return false
}

func invalidRepoField(r *Repo, field, reason string) error {
	return errors.New(errors.ErrTypeValidation,
		fmt.Sprintf("repo %q: invalid %s: %s", r.Name, field, reason)).
		WithOperation("validateRepo").
		WithContext("field", field)
}
