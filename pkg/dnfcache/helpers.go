package dnfcache

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
)

// StripRPMConstraint strips RPM version constraint suffixes from a dep name.
// Examples: "glibc >= 2.17" → "glibc", "libfoo(x86-64)" → "libfoo(x86-64)"
// (parenthesised capability names are kept as-is).
func StripRPMConstraint(name string) string {
	name = strings.TrimSpace(name)

	// RPM constraints use space-separated operators: "name >= ver"
	if before, _, ok := strings.Cut(name, " "); ok {
		return strings.TrimSpace(before)
	}

	return name
}

// loadInstalledSet returns the set of package names currently installed
// according to the RPM database. On hosts where the SQLite rpmdb is not
// available (Rocky 8 / BerkeleyDB), falls back to "rpm -qa".
func loadInstalledSet(ctx context.Context) map[string]bool {
	db, err := rpmdb.Open()
	if err == nil {
		names, err := db.ListInstalled(ctx)
		if err == nil {
			set := make(map[string]bool, len(names))
			for _, name := range names {
				set[name] = true
			}

			return set
		}
	}

	if !errors.Is(err, rpmdb.ErrLegacyDB) && err != nil {
		return map[string]bool{}
	}

	logger.Debug("dnfcache: legacy BDB rpmdb, falling back to rpm -qa")

	return loadInstalledSetSubprocess(ctx)
}

// loadInstalledSetSubprocess returns the set of package names currently
// installed using the rpm -qa subprocess. Used as fallback for legacy
// BerkeleyDB hosts.
func loadInstalledSetSubprocess(ctx context.Context) map[string]bool {
	out, err := exec.CommandContext(
		ctx,
		"rpm",
		"-qa",
		"--queryformat",
		"%{NAME}\n",
	).Output() // #nosec G204
	if err != nil {
		return map[string]bool{}
	}

	set := make(map[string]bool)

	for line := range strings.SplitSeq(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			set[name] = true
		}
	}

	return set
}

// loadInstalledProvides returns the set of capabilities (Provides) currently
// satisfied by installed packages. This includes virtual package names like
// "coreutils" which may be provided by "coreutils-single" on minimal images.
// Used by ResolveDeps to avoid installing alternative packages that conflict
// with what is already on the system.
func loadInstalledProvides(ctx context.Context) map[string]bool {
	db, err := rpmdb.Open()
	if err == nil {
		provides, err := db.ListInstalledProvides(ctx)
		if err == nil {
			set := make(map[string]bool, len(provides))
			for _, prov := range provides {
				capName := StripRPMConstraint(prov)
				if capName != "" {
					set[capName] = true
				}
			}

			return set
		}
	}

	if !errors.Is(err, rpmdb.ErrLegacyDB) && err != nil {
		return map[string]bool{}
	}

	logger.Debug("dnfcache: legacy BDB rpmdb, falling back to rpm -qa")

	return loadInstalledProvidesSubprocess(ctx)
}

// loadInstalledProvidesSubprocess returns the set of capabilities (Provides)
// currently satisfied by installed packages using the rpm -qa subprocess.
// Used as fallback for legacy BerkeleyDB hosts.
func loadInstalledProvidesSubprocess(ctx context.Context) map[string]bool {
	out, err := exec.CommandContext(
		ctx,
		"rpm",
		"-qa",
		"--queryformat",
		"[%{PROVIDENAME}\n]",
	).Output() // #nosec G204
	if err != nil {
		return map[string]bool{}
	}

	set := make(map[string]bool)

	for line := range strings.SplitSeq(string(out), "\n") {
		capName := StripRPMConstraint(strings.TrimSpace(line))
		if capName != "" {
			set[capName] = true
		}
	}

	return set
}

// expandRepoVars replaces $basearch, $releasever, and any other $var
// placeholders found in /etc/dnf/vars/ (e.g. $infra, $contentdir used by
// EPEL metalink URLs).
//
// $basearch maps the Go GOARCH to the RPM architecture string.
// $releasever is read from /etc/os-release (VERSION_ID field).
// All other $var tokens are resolved from /etc/dnf/vars/<var>; if the file
// is absent the placeholder is left unexpanded.
func expandRepoVars(rawURL string) string {
	rawURL = strings.ReplaceAll(rawURL, "$basearch", goArchToRPM())
	rawURL = strings.ReplaceAll(rawURL, "$releasever", readReleasever())
	rawURL = expandDNFVars(rawURL)

	return normalizeURL(rawURL)
}

// normalizeURL collapses double slashes in the path component of a URL.
// Some Rocky Linux / EPEL mirror list entries contain paths like
// "/pub/rocky//8.10/..." where variable substitution produces "//".
// net/url.Parse + String() round-trips the URL and cleans the path.
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// url.Parse does not collapse // in the path; do it manually.
	for strings.Contains(u.Path, "//") {
		u.Path = strings.ReplaceAll(u.Path, "//", "/")
	}

	return u.String()
}

// expandDNFVars replaces any remaining $var tokens in rawURL by reading
// /etc/dnf/vars/<var>. Unknown vars are left as-is.
func expandDNFVars(rawURL string) string {
	return dnfVarRe.ReplaceAllStringFunc(rawURL, func(m string) string {
		varName := m[1:] // strip leading '$'

		val, err := os.ReadFile("/etc/dnf/vars/" + varName) // #nosec G304
		if err != nil {
			return m // leave unexpanded
		}

		return strings.TrimSpace(string(val))
	})
}

var dnfVarRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

const (
	archX8664   = "x86_64"
	archPPC64LE = "ppc64le"
	archS390X   = "s390x"
)

// goArchToRPM maps GOARCH values to RPM $basearch strings.
func goArchToRPM() string {
	switch runtime.GOARCH {
	case "amd64":
		return archX8664
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armhfp"
	case "ppc64le": //nolint:goconst
		return archPPC64LE
	case "s390x": //nolint:goconst
		return archS390X
	default:
		return runtime.GOARCH
	}
}

// readReleasever reads VERSION_ID from /etc/os-release.
// Returns an empty string if the file cannot be read or the field is absent.
func readReleasever() string {
	data, err := os.ReadFile("/etc/os-release") // #nosec G304
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "VERSION_ID" {
			continue
		}

		return strings.Trim(strings.TrimSpace(val), `"'`)
	}

	return ""
}
