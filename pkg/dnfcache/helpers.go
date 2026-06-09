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
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
)

// StripRPMConstraint strips RPM version constraint suffixes from a dep name.
// Examples: "glibc >= 2.17" → "glibc", "libfoo(x86-64)" → "libfoo(x86-64)"
// (parenthesised capability names are kept as-is).
func StripRPMConstraint(name string) string {
	name = strings.TrimSpace(name)

	// Boolean/rich deps are kept verbatim so the caller can dispatch to
	// ExpandBooleanDep. They start with '(' and contain keywords like
	// "if", "or", "and" — splitting on space here would corrupt them.
	if strings.HasPrefix(name, "(") {
		return name
	}

	// RPM constraints use space-separated operators: "name >= ver"
	if before, _, ok := strings.Cut(name, " "); ok {
		return strings.TrimSpace(before)
	}

	return name
}

// booleanDepKeywords are the operator tokens used by RPM rich/boolean deps.
// Reference: http://rpm.org/user_doc/boolean_dependencies.html
var booleanDepKeywords = map[string]bool{
	"and": true, "or": true, "if": true, "else": true,
	"unless": true, "with": true, "without": true, "then": true,
}

// IsBooleanDep reports whether name is an RPM rich/boolean dependency
// expression like "(foo if bar)" or "(libA or libB)".
func IsBooleanDep(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "(")
}

// ExpandBooleanDep extracts the candidate package names from an RPM
// rich/boolean dependency expression. Boolean operators (if/or/and/…) and
// version constraints are stripped; the remaining tokens are returned as
// soft (best-effort) dependencies. Returns nil for non-boolean inputs.
//
// Example: "(gcc-plugin-annobin if gcc)" → ["gcc-plugin-annobin", "gcc"]
//
// The resolver visits each result as a soft dep, so missing tokens (e.g.
// the conditional trigger that isn't actually being installed) are logged
// and ignored rather than reported as build-breaking unresolved deps.
func ExpandBooleanDep(name string) []string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "(") {
		return nil
	}

	// Strip the outermost parentheses; nested ones are flattened by the
	// tokenizer below since we only care about identifier tokens.
	name = strings.TrimPrefix(name, "(")
	name = strings.TrimSuffix(name, ")")

	// Replace structural punctuation with spaces so a simple Fields split
	// yields candidate tokens. Comparison operators (>=, <=, =, >, <) are
	// dropped together with their version operands by the keyword/operand
	// filter below.
	for _, ch := range []string{"(", ")", ","} {
		name = strings.ReplaceAll(name, ch, " ")
	}

	var (
		result   []string
		skipNext bool
	)

	for tok := range strings.FieldsSeq(name) {
		if skipNext {
			skipNext = false
			continue
		}

		// Version comparison operators: the following token is a version,
		// not a package name.
		if tok == ">=" || tok == "<=" || tok == "=" || tok == ">" || tok == "<" || tok == "==" {
			skipNext = true
			continue
		}

		if booleanDepKeywords[strings.ToLower(tok)] {
			continue
		}

		// rpmlib() feature deps are not real packages.
		if strings.HasPrefix(tok, "rpmlib(") {
			continue
		}

		result = append(result, tok)
	}

	return result
}

// loadInstalledSet returns the set of package names currently installed
// according to the RPM database. On hosts where the SQLite rpmdb is not
// available (Rocky 8 / BerkeleyDB), reads the BDB Packages database
// natively; "rpm -qa" remains the last-resort fallback.
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

	logger.Debug(i18n.T("logger.dnfcache.debug.legacy_bdb_rpmdb_falling"))

	if legacy, err := rpmdb.OpenLegacy(); err == nil {
		if names, err := legacy.ListInstalled(ctx); err == nil {
			set := make(map[string]bool, len(names))
			for _, name := range names {
				set[name] = true
			}

			return set
		}
	}

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
	).Output()
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
//
// On legacy BerkeleyDB hosts the BDB Packages database is read natively;
// "rpm -qa" remains the last-resort fallback.
func loadInstalledProvides(ctx context.Context) map[string]bool {
	db, err := rpmdb.Open()
	if err == nil {
		provides, err := db.ListInstalledProvides(ctx)
		if err == nil {
			return providesSet(provides)
		}
	}

	if !errors.Is(err, rpmdb.ErrLegacyDB) && err != nil {
		return map[string]bool{}
	}

	logger.Debug(i18n.T("logger.dnfcache.debug.legacy_bdb_rpmdb_falling"))

	if legacy, err := rpmdb.OpenLegacy(); err == nil {
		if provides, err := legacy.ListInstalledProvides(ctx); err == nil {
			return providesSet(provides)
		}
	}

	return loadInstalledProvidesSubprocess(ctx)
}

// providesSet normalizes a Provides list into a constraint-stripped set.
func providesSet(provides []string) map[string]bool {
	set := make(map[string]bool, len(provides))

	for _, prov := range provides {
		capName := StripRPMConstraint(prov)
		if capName != "" {
			set[capName] = true
		}
	}

	return set
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
	).Output()
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

// dnfVarCache memoizes /etc/dnf/vars/<var> lookups (including misses) so
// repeated URL expansion doesn't re-stat the filesystem per repo/package.
var dnfVarCache sync.Map // varName → string (expanded value, or "$"+varName on miss)

// expandDNFVars replaces any remaining $var tokens in rawURL by reading
// /etc/dnf/vars/<var>. Unknown vars are left as-is. Values are cached for
// the process lifetime — dnf vars are static host configuration.
func expandDNFVars(rawURL string) string {
	return dnfVarRe.ReplaceAllStringFunc(rawURL, func(m string) string {
		if cached, ok := dnfVarCache.Load(m); ok {
			return cached.(string)
		}

		varName := m[1:] // strip leading '$'

		expanded := m // leave unexpanded on miss

		if val, err := os.ReadFile("/etc/dnf/vars/" + varName); err == nil { //nolint:gosec
			expanded = strings.TrimSpace(string(val))
		}

		dnfVarCache.Store(m, expanded)

		return expanded
	})
}

var dnfVarRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

const (
	archX8664   = "x86_64"
	archPPC64LE = "ppc64le"
	archS390X   = "s390x"
	archI686    = "i686"
)

// goArchToRPM maps GOARCH values to RPM $basearch strings.
func goArchToRPM() string {
	switch runtime.GOARCH {
	case "amd64":
		return archX8664
	case "arm64":
		return "aarch64"
	case "386":
		return archI686
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

// readReleasever returns VERSION_ID from /etc/os-release, cached for the
// process lifetime (the file is static host configuration and this is
// called for every URL expansion).
// Returns an empty string if the file cannot be read or the field is absent.
var readReleasever = sync.OnceValue(func() string {
	data, err := os.ReadFile("/etc/os-release")
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
})
