package nfpm

import (
	"regexp"
	"strings"
)

// semverPrefixRe matches a leading "Major.Minor.Patch" numeric triple, the
// only shape SplitVersion treats as semver.
var semverPrefixRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// SplitVersion parses "v1.2.3-beta.1+deadbeef" into ("1.2.3","beta.1","deadbeef").
// Leading "v" is stripped. When the string is not semver-shaped (its leading
// segment doesn't match ^\d+\.\d+\.\d+) it returns (version, "", "")
// unchanged. Only applied when VersionSchema is "" or "semver".
func SplitVersion(v string) (version, prerelease, metadata string) {
	v = strings.TrimPrefix(v, "v")

	if !semverPrefixRe.MatchString(v) {
		return v, "", ""
	}

	version = v

	if idx := strings.Index(version, "+"); idx >= 0 {
		metadata = version[idx+1:]
		version = version[:idx]
	}

	triple := semverPrefixRe.FindString(version)
	rest := version[len(triple):]

	if pre, post, found := strings.Cut(rest, "-"); found {
		prerelease = post
		version = triple + pre
	}

	return version, prerelease, metadata
}
