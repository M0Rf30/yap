package dnfcache

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
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
	out, err := exec.CommandContext(ctx, "rpm", "-qa", "--queryformat", "%{NAME}\n").Output() // #nosec G204
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

// expandRepoVars replaces $basearch, $releasever, and any other $var
// placeholders found in /etc/dnf/vars/ (e.g. $infra, $contentdir used by
// EPEL metalink URLs).
//
// $basearch maps the Go GOARCH to the RPM architecture string.
// $releasever is read from /etc/os-release (VERSION_ID field).
// All other $var tokens are resolved from /etc/dnf/vars/<var>; if the file
// is absent the placeholder is left unexpanded.
func expandRepoVars(url string) string {
	url = strings.ReplaceAll(url, "$basearch", goArchToRPM())
	url = strings.ReplaceAll(url, "$releasever", readReleasever())
	url = expandDNFVars(url)

	return url
}

// expandDNFVars replaces any remaining $var tokens in url by reading
// /etc/dnf/vars/<var>. Unknown vars are left as-is.
func expandDNFVars(url string) string {
	return dnfVarRe.ReplaceAllStringFunc(url, func(m string) string {
		varName := m[1:] // strip leading '$'

		val, err := os.ReadFile("/etc/dnf/vars/" + varName) // #nosec G304
		if err != nil {
			return m // leave unexpanded
		}

		return strings.TrimSpace(string(val))
	})
}

var dnfVarRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// goArchToRPM maps GOARCH values to RPM $basearch strings.
func goArchToRPM() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armhfp"
	case "ppc64le":
		return "ppc64le"
	case "s390x":
		return "s390x"
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
