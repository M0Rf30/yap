// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	pkgrpm "github.com/M0Rf30/pkgformat/rpm"
	rpmtag "github.com/M0Rf30/pkgformat/rpm/tag"

	"github.com/M0Rf30/yap/v2/pkg/yapdb"
)

// extractCapabilities extracts Provides, Requires, Conflicts, and Obsoletes
// from an RPM header and returns them as a slice of yapdb.Capability.
func extractCapabilities(rpm *pkgrpm.Rpm) []yapdb.Capability {
	var caps []yapdb.Capability

	caps = append(caps, readCapList(rpm, "provide",
		rpmtag.PROVIDENAME, rpmtag.PROVIDEFLAGS, rpmtag.PROVIDEVERSION)...)
	caps = append(caps, readCapList(rpm, "require",
		rpmtag.REQUIRENAME, rpmtag.REQUIREFLAGS, rpmtag.REQUIREVERSION)...)
	caps = append(caps, readCapList(rpm, "conflict",
		rpmtag.CONFLICTNAME, rpmtag.CONFLICTFLAGS, rpmtag.CONFLICTVERSION)...)
	caps = append(caps, readCapList(rpm, "obsolete",
		rpmtag.OBSOLETENAME, rpmtag.OBSOLETEFLAGS, rpmtag.OBSOLETEVERSION)...)

	return caps
}

// readCapList reads a capability list (names, flags, versions) from the RPM header
// and returns them as a slice of yapdb.Capability with the given kind.
// The three arrays are expected to be parallel (same length or shorter versions/flags).
func readCapList(rpm *pkgrpm.Rpm, kind string, nameTag, flagsTag, versionTag int) []yapdb.Capability {
	names, _ := rpm.Header.GetStrings(nameTag)
	if len(names) == 0 {
		return nil
	}

	flags, _ := rpm.Header.GetUint32s(flagsTag)
	versions, _ := rpm.Header.GetStrings(versionTag)

	out := make([]yapdb.Capability, len(names))
	for i := range names {
		var (
			fl  int
			ver string
		)

		if i < len(flags) {
			fl = int(flags[i])
		}

		if i < len(versions) {
			ver = versions[i]
		}

		out[i] = yapdb.Capability{
			Kind:    kind,
			Name:    names[i],
			Flags:   fl,
			Version: ver,
		}
	}

	return out
}
